package native

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/waveform"
)

func TestNativeCompressedDecoder(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for native integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adapter := Adapter{Executable: helper, SilentPreview: true}
	decoder, err := NewDecoder(ctx, adapter)
	if err != nil {
		t.Fatal(err)
	}
	formats := decoder.Formats()
	formats[0] = "mutated"
	if decoder.Formats()[0] != "wav-pcm" {
		t.Fatal("borrowed format capabilities")
	}
	for _, codec := range []string{"flac", "mp3"} {
		t.Run(codec, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../native/testdata", "generated-stereo."+codec))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "音楽-guitär.bin")
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			r, err := (filesystem.Host{}).Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			info, err := decoder.Inspect(ctx, r)
			if err != nil || info.Encoding != codec || info.Frames != 48000 || info.SampleRate != 48000 || info.Channels != 2 {
				t.Fatal(info, err)
			}
			stream, err := decoder.OpenPCM(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			all := make([]float32, 96000)
			for frame := 0; frame < 48000; {
				n, err := stream.ReadFrames(ctx, all[frame*2:min(frame+8192, 48000)*2])
				if err != nil || n < 1 {
					t.Fatal(n, err)
				}
				frame += n
			}
			if n, err := stream.ReadFrames(ctx, make([]float32, 2)); n != 0 || err != io.EOF {
				t.Fatal("PCM EOF", n, err)
			}
			if err = stream.SeekFrame(ctx, 12345); err != nil {
				t.Fatal(err)
			}
			seek := make([]float32, 1024*2)
			if n, err := stream.ReadFrames(ctx, seek); n != 1024 || err != nil || !reflect.DeepEqual(seek, all[12345*2:(12345+1024)*2]) {
				t.Fatal("worker seek differs", n, err)
			}
			c, cancel := context.WithCancel(ctx)
			cancel()
			if !errors.Is(stream.SeekFrame(c, 0), context.Canceled) {
				t.Fatal("seek ignored cancellation")
			}
			peaks, err := decoder.Peaks(ctx, r, 17)
			if err != nil || peaks.Frames != 48000 || len(peaks.Channels) != 2 || len(peaks.Channels[0]) != 17 {
				t.Fatal(peaks, err)
			}

			detailCursor, err := decoder.OpenPCM(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			detail, err := waveform.NewDetail(info, detailCursor)
			if err != nil {
				detailCursor.Close()
				t.Fatal(err)
			}
			for _, bounds := range [][2]int64{{1, 4}, {1151, 1154}, {12345, 12348}, {47997, 48000}} {
				view, err := detail.Query(ctx, bounds[0], bounds[1], 4096)
				if err != nil || view.FramesPerCell != 1 || len(view.Cells) != 3 {
					t.Fatal(view, err)
				}
				for j, cell := range view.Cells {
					for channel, peak := range cell.Channels {
						expected := all[(int(bounds[0])+j)*2+channel]
						if peak.Min != expected || peak.Max != expected {
							t.Fatal("codec detail changed source samples", codec, cell, expected)
						}
					}
				}
			}
			if err = detail.Close(); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "render.wav")
			if err = adapter.Render(ctx, engine.RenderRequest{Source: path, Destination: out, StartFrame: 12000, EndFrame: 36000, Speed: .75, Pitch: 3}); err != nil {
				t.Fatal(err)
			}
			rendered, err := filesystem.Inspect(ctx, out)
			if err != nil || rendered.Frames != 32000 || rendered.Channels != 2 {
				t.Fatal(rendered, err)
			}
			report, err := adapter.Preview(ctx, engine.PreviewRequest{Source: path, StartFrame: 12000, EndFrame: 36000, Speed: .75, Pitch: 3})
			if err != nil || report.CopiedFrames != 32000 || report.OutputFrames != 32000 {
				t.Fatal(report, err)
			}
			after, err := decoder.Inspect(ctx, r)
			if err != nil || after.SHA256 != info.SHA256 {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestNativeCompressedDecoderKeepsBorrowedSourceAfterPathReplacement(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for native integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	decoder, err := NewDecoder(ctx, Adapter{Executable: helper})
	if err != nil {
		t.Fatal(err)
	}
	for _, codec := range []string{"flac", "mp3"} {
		t.Run(codec, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("../../native/testdata", "generated-stereo."+codec))
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "原本."+codec)
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			r, err := (filesystem.Host{}).Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			info, err := decoder.Inspect(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			file, ok := filesystem.BorrowedFile(r)
			if !ok {
				t.Fatal("filesystem reader did not lend its original file")
			}
			if _, err = file.Seek(123, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			baseline, err := decoder.OpenPCM(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if err = baseline.SeekFrame(ctx, 12345); err != nil {
				t.Fatal(err)
			}
			expected := make([]float32, 2048)
			if n, err := baseline.ReadFrames(ctx, expected); err != nil || n != 1024 {
				t.Fatal(n, err)
			}
			if err = baseline.Close(); err != nil {
				t.Fatal(err)
			}
			if position, err := file.Seek(0, io.SeekCurrent); err != nil || position != 123 {
				t.Fatal("native decoder changed original file position", position, err)
			}
			if err = os.Rename(path, filepath.Join(dir, "moved."+codec)); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("replacement is not the original audio"), 0600); err != nil {
				t.Fatal(err)
			}
			current, err := decoder.Inspect(ctx, r)
			if err != nil || !reflect.DeepEqual(current, info) {
				t.Fatal("metadata/hash followed replacement path", current, err)
			}
			stream, err := decoder.OpenPCM(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if err = stream.SeekFrame(ctx, 12345); err != nil {
				t.Fatal(err)
			}
			actual := make([]float32, 2048)
			if n, err := stream.ReadFrames(ctx, actual); err != nil || n != 1024 || !reflect.DeepEqual(actual, expected) {
				t.Fatal("PCM followed replacement path", n, err)
			}
			if err = stream.Close(); err != nil {
				t.Fatal(err)
			}
			if position, err := file.Seek(0, io.SeekCurrent); err != nil || position != 123 {
				t.Fatal("inherited reader changed original cursor", position, err)
			}
			prefix := make([]byte, 4)
			if _, err = r.ReadAt(prefix, 0); err != nil || !reflect.DeepEqual(prefix, data[:4]) {
				t.Fatal("helper closed or replaced original resource", prefix, err)
			}
		})
	}
}
