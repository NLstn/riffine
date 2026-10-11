package waveform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

type source struct{ *bytes.Reader }

func (source) Name() string { return "generated.wav" }
func (source) Close() error { return nil }

type memoryCache struct {
	data map[string][]byte
	puts int
}

func (c *memoryCache) Get(ctx context.Context, key string, max int) ([]byte, error) {
	data, ok := c.data[key]
	if !ok {
		return nil, io.EOF
	}
	if len(data) > max {
		return nil, errors.New("oversized")
	}
	return append([]byte(nil), data...), ctx.Err()
}
func (c *memoryCache) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.data[key] = append([]byte(nil), data...)
	c.puts++
	return nil
}
func fixture(t *testing.T, hz float64) source {
	t.Helper()
	var b bytes.Buffer
	if err := audio.WriteTone(&b, .2, hz, 48000, 2); err != nil {
		t.Fatal(err)
	}
	return source{bytes.NewReader(b.Bytes())}
}
func TestPyramidWindowCacheAndCoordinates(t *testing.T) {
	ctx := context.Background()
	r := fixture(t, 440)
	cache := &memoryCache{data: map[string][]byte{}}
	var progress []Progress
	result, err := Open(ctx, r, audio.WAVDecoder{}, cache, func(p Progress) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}
	if result.CacheStatus != "rebuilt" || result.CacheError != nil || cache.puts != 1 || progress[0].DecodedFrames != 0 || progress[len(progress)-1].DecodedFrames != 9600 {
		t.Fatal(result, progress)
	}
	p := result.Pyramid
	// Base-level cells are source-aligned; final cell contains only actual frames.
	w, err := p.Query(ctx, 0, 9600, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if w.FramesPerCell != 256 || len(w.Cells) != 38 || w.Cells[37].StartFrame != 9472 || w.Cells[37].EndFrame != 9600 || w.SourceFrames != 9600 {
		t.Fatal(w)
	}
	for _, cell := range w.Cells {
		if len(cell.Channels) != 2 || cell.Channels[0] != cell.Channels[1] || cell.Channels[0].Min > -.24 || cell.Channels[0].Max < .24 {
			t.Fatal(cell)
		}
	}
	for _, bins := range []int{1, 2, 3, 7, 37, 4096} {
		for _, window := range [][2]int64{{0, 9600}, {1, 9599}, {255, 257}, {1234, 1235}} {
			view, err := p.Query(ctx, window[0], window[1], bins)
			if err != nil || len(view.Cells) > bins || view.RequestedStart != window[0] || view.RequestedEnd != window[1] || view.Cells[0].StartFrame > window[0] || view.Cells[len(view.Cells)-1].EndFrame < window[1] {
				t.Fatal(view, err)
			}
		}
	}
	hit, err := Open(ctx, r, audio.WAVDecoder{}, cache, func(Progress) { t.Fatal("cache hit decoded audio") })
	if err != nil || hit.CacheStatus != "hit" || cache.puts != 1 {
		t.Fatal(hit, err)
	}
	again, _ := hit.Pyramid.Query(ctx, 0, 9600, 4096)
	if !reflect.DeepEqual(w, again) {
		t.Fatal("cache changed waveform")
	}
	w.Cells[0].Channels[0].Min = 99
	unchanged, _ := p.Query(ctx, 0, 9600, 4096)
	if unchanged.Cells[0].Channels[0].Min == 99 {
		t.Fatal("query leaked mutable pyramid storage")
	}
}
func TestCacheCorruptionAndSourceReplacement(t *testing.T) {
	ctx := context.Background()
	r := fixture(t, 440)
	cache := &memoryCache{data: map[string][]byte{}}
	result, err := Open(ctx, r, audio.WAVDecoder{}, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	k := key(result.Pyramid.info)
	valid := append([]byte(nil), cache.data[k]...)
	for _, offset := range []int{0, 8, 12, 36, 68, 72, 76, 80, len(valid) - 1} {
		cache.data[k] = append([]byte(nil), valid...)
		cache.data[k][offset] ^= 1
		result, err = Open(ctx, r, audio.WAVDecoder{}, cache, nil)
		if err != nil || result.CacheStatus != "rebuilt" || !bytes.Equal(cache.data[k], valid) {
			t.Fatal("corrupt cache was not rebuilt", offset, err)
		}
	}
	cache.data[k] = append(valid, 1)
	result, err = Open(ctx, r, audio.WAVDecoder{}, cache, nil)
	if err != nil || result.CacheStatus != "rebuilt" {
		t.Fatal("trailing cache data accepted")
	}
	replacement, err := Open(ctx, fixture(t, 660), audio.WAVDecoder{}, cache, nil)
	if err != nil || replacement.CacheStatus != "rebuilt" || replacement.Pyramid.info.SHA256 == result.Pyramid.info.SHA256 || len(cache.data) != 2 {
		t.Fatal(replacement, err)
	}
}
func TestCancelledGenerationNeverPublishes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cache := &memoryCache{data: map[string][]byte{}}
	result, err := Open(ctx, fixture(t, 440), audio.WAVDecoder{}, cache, func(p Progress) {
		if p.DecodedFrames > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || result.Pyramid != nil || cache.puts != 0 {
		t.Fatal(result, err)
	}
}

// A synthetic decoder simulates arbitrarily long source metadata while reads
// remain fixed blocks. This avoids allocating an enormous test audio file.
type synthetic struct {
	info   audio.Info
	closed *bool
	bad    bool
}

func (d synthetic) Formats() []string                                            { return []string{"synthetic-test"} }
func (d synthetic) Inspect(context.Context, resource.Reader) (audio.Info, error) { return d.info, nil }
func (d synthetic) Peaks(context.Context, resource.Reader, int) (audio.Waveform, error) {
	return audio.Waveform{}, errors.New("unused")
}
func (d synthetic) OpenPCM(context.Context, resource.Reader) (audio.PCMStream, error) {
	return &syntheticStream{remaining: d.info.Frames, channels: d.info.Channels, closed: d.closed, bad: d.bad}, nil
}

type syntheticStream struct {
	remaining int64
	channels  int
	closed    *bool
	bad       bool
}

func (s *syntheticStream) ReadFrames(ctx context.Context, b []float32) (int, error) {
	n := int(min(s.remaining, int64(len(b)/s.channels)))
	for j := 0; j < n*s.channels; j++ {
		b[j] = 0
	}
	if s.bad && n > 0 {
		b[0] = float32(math.NaN())
	}
	s.remaining -= int64(n)
	return n, ctx.Err()
}
func (s *syntheticStream) SeekFrame(context.Context, int64) error { return errors.New("unused") }
func (s *syntheticStream) Close() error                           { *s.closed = true; return nil }
func TestLongSourceMemoryBoundAndSilence(t *testing.T) {
	hash := sha256.Sum256([]byte("synthetic"))
	info := audio.Info{SHA256: hex.EncodeToString(hash[:]), Frames: 48000 * 1800, SampleRate: 48000, Channels: 2}
	closed := false
	result, err := Open(context.Background(), source{bytes.NewReader(nil)}, synthetic{info: info, closed: &closed}, nil, nil)
	if err != nil || !closed {
		t.Fatal(err)
	}
	p := result.Pyramid
	total := 0
	for _, level := range p.levels {
		total += len(level[0]) * info.Channels
	}
	if len(p.levels[0][0]) > MaxBaseBins || total > MaxBaseBins*4 || len(encode(p)) > MaxCacheBytes {
		t.Fatal("pyramid storage was not bounded", total)
	}
	view, err := p.Query(context.Background(), 100000, info.Frames-100000, 997)
	if err != nil || len(view.Cells) > 997 {
		t.Fatal(err)
	}
	for _, cell := range view.Cells {
		for _, peak := range cell.Channels {
			if peak.Min != 0 || peak.Max != 0 {
				t.Fatal("silence peak", peak)
			}
		}
	}
	closed = false
	_, err = Open(context.Background(), source{bytes.NewReader(nil)}, synthetic{info: info, closed: &closed, bad: true}, nil, nil)
	if err == nil || !closed {
		t.Fatal("non-finite source accepted or stream leaked")
	}
}
