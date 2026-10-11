package waveform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/audio"
)

type detailPCM struct {
	frame   int64
	frames  int64
	decoded int64
	seeks   []int64
	closed  bool
	bad     bool
	cancel  context.CancelFunc
}

func (s *detailPCM) SeekFrame(ctx context.Context, frame int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.frame = frame
	s.seeks = append(s.seeks, frame)
	return nil
}
func (s *detailPCM) ReadFrames(ctx context.Context, dst []float32) (int, error) {
	n := int(min(int64(len(dst)/2), s.frames-s.frame))
	for j := 0; j < n; j++ {
		// Stereo, clipped impulses and silence at exact source coordinates.
		v := float32(0)
		if (s.frame+int64(j))%7 == 0 {
			v = 1.5
		}
		dst[j*2] = v
		dst[j*2+1] = -v
	}
	if s.bad && n > 0 {
		dst[0] = float32(math.NaN())
	}
	s.frame += int64(n)
	s.decoded += int64(n)
	if s.cancel != nil {
		s.cancel()
	}
	return n, nil
}
func (s *detailPCM) Close() error { s.closed = true; return nil }
func detailInfo(frames int64) audio.Info {
	hash := sha256.Sum256([]byte("generated detail source"))
	return audio.Info{Encoding: "wav-pcm", Bits: 16, SampleRate: 48000, Channels: 2, Frames: frames, SHA256: hex.EncodeToString(hash[:])}
}
func TestLongSourceExactDetailReadsOnlyViewport(t *testing.T) {
	ctx := context.Background()
	// Huge source; this port deliberately has no storage or whole-source scan.
	stream := &detailPCM{frames: 1 << 40}
	detail, err := NewDetail(detailInfo(stream.frames), stream)
	if err != nil {
		t.Fatal(err)
	}
	defer detail.Close()
	start := int64(1<<39) + 255
	window, err := detail.Query(ctx, start, start+3, 4096)
	if err != nil || window.FramesPerCell != 1 || len(window.Cells) != 3 || stream.decoded != 3 || !reflect.DeepEqual(stream.seeks, []int64{start}) {
		t.Fatal(window, stream, err)
	}
	for j, cell := range window.Cells {
		if cell.StartFrame != start+int64(j) || cell.EndFrame != cell.StartFrame+1 || cell.Channels[0].Min != cell.Channels[0].Max || cell.Channels[1].Min != -cell.Channels[0].Max {
			t.Fatal(cell)
		}
	}
	window.Cells[0].Channels[0].Min = 99
	again, err := detail.Query(ctx, start, start+3, 2)
	if err != nil || again.FramesPerCell != 2 || len(again.Cells) != 2 || again.Cells[0].StartFrame != start || again.Cells[1].EndFrame != start+3 || stream.decoded != 6 {
		t.Fatal(again, err)
	}
	for _, cell := range again.Cells {
		if cell.Channels[0].Min == 99 || cell.Channels[0].Min > cell.Channels[0].Max {
			t.Fatal("mutated detail storage", cell)
		}
	}
}
func TestDetailBoundsCancellationAndFailures(t *testing.T) {
	ctx := context.Background()
	stream := &detailPCM{frames: MaxDetailFrames + 1}
	detail, _ := NewDetail(detailInfo(stream.frames), stream)
	defer detail.Close()
	for _, request := range [][3]int64{{-1, 2, 1}, {0, 0, 1}, {0, MaxDetailFrames + 1, 1}, {0, 1, 0}, {0, 1, 4097}, {0, stream.frames + 1, 1}} {
		if _, err := detail.Query(ctx, request[0], request[1], int(request[2])); err == nil {
			t.Fatal("invalid detail accepted", request)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := detail.Query(cancelled, 0, 1, 1); !errors.Is(err, context.Canceled) || len(stream.seeks) != 0 {
		t.Fatal("rejected request touched decoder", err)
	}
	stream.cancel = nil
	if _, err := detail.Query(ctx, stream.frames-1, stream.frames, 1); err != nil || stream.decoded != 1 {
		t.Fatal("last source frame", err)
	}
	readCtx, readCancel := context.WithCancel(ctx)
	stream.cancel = readCancel
	if _, err := detail.Query(readCtx, 0, 10000, 7); !errors.Is(err, context.Canceled) {
		t.Fatal("partial cancellation succeeded", err)
	}
	if _, err := detail.Query(ctx, 0, 1, 1); err == nil {
		t.Fatal("failed cursor reused")
	}
	if err := detail.Close(); err != nil || !stream.closed {
		t.Fatal(err)
	}
	if _, err := detail.Query(ctx, 0, 1, 1); err == nil {
		t.Fatal("closed cursor queried")
	}
	for _, bad := range []bool{false, true} {
		broken := &detailPCM{frames: 0, bad: bad}
		if bad {
			broken.frames = 2
		}
		handle, _ := NewDetail(detailInfo(10), broken)
		if _, err := handle.Query(ctx, 0, 2, 1); err == nil {
			t.Fatal("truncated/non-finite source accepted")
		}
		handle.Close()
	}
}
func TestDetailGoldenPCMMatchesDecodedSamples(t *testing.T) {
	ctx := context.Background()
	source := fixture(t, 440)
	info, err := (audio.WAVDecoder{}).Inspect(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := (audio.WAVDecoder{}).OpenPCM(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	defer pcm.Close()
	samples := make([]float32, 8192*2)
	if n, err := pcm.ReadFrames(ctx, samples); n != 8192 || err != nil {
		t.Fatal(n, err)
	}
	cursor, _ := (audio.WAVDecoder{}).OpenPCM(ctx, source)
	detail, _ := NewDetail(info, cursor)
	defer detail.Close()
	for _, bounds := range [][2]int64{{255, 258}, {4095, 4100}, {8191, 8192}} {
		window, err := detail.Query(ctx, bounds[0], bounds[1], 4096)
		if err != nil {
			t.Fatal(err)
		}
		for j, cell := range window.Cells {
			for channel, peak := range cell.Channels {
				want := samples[(int(bounds[0])+j)*2+channel]
				if peak.Min != want || peak.Max != want {
					t.Fatal("detail sample differs", cell, want)
				}
			}
		}
	}
	if err := source.Close(); err != nil && err != io.EOF {
		t.Fatal(err)
	}
}
