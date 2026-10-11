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
