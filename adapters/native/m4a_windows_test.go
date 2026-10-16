package native

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/engine"
)

func TestWindowsM4A(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for Windows M4A integration")
	}
	for _, fixture := range []struct{ suffix, encoding string }{{"m4a", "aac"}, {"alac.m4a", "alac"}} {
		t.Run(fixture.encoding, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			adapter := Adapter{Executable: helper, SilentPreview: true}
			decoder, err := NewDecoder(ctx, adapter)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join("../../native/testdata", "generated-stereo."+fixture.suffix))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "音楽.M4A")
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			reader, err := (filesystem.Host{}).Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			info, err := decoder.Inspect(ctx, reader)
			if err != nil || info.Encoding != fixture.encoding || info.Channels != 2 || info.SampleRate != 48000 || info.Frames < 48000 || info.Frames > 50176 {
				t.Fatal("M4A metadata", info, err)
			}
			stream, err := decoder.OpenPCM(ctx, reader)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			pcm := make([]float32, int(info.Frames)*2)
			for frame := 0; frame < int(info.Frames); {
				n, err := stream.ReadFrames(ctx, pcm[frame*2:min(frame+8192, int(info.Frames))*2])
				if err != nil || n == 0 {
					t.Fatal("M4A PCM", n, err)
				}
				frame += n
			}
			if n, err := stream.ReadFrames(ctx, make([]float32, 2)); n != 0 || err != io.EOF {
				t.Fatal("M4A EOF", n, err)
			}
			if err = stream.SeekFrame(ctx, 12345); err != nil {
				t.Fatal(err)
			}
			seek := make([]float32, 2048)
			if n, err := stream.ReadFrames(ctx, seek); n != 1024 || err != nil || !reflect.DeepEqual(seek, pcm[12345*2:(12345+1024)*2]) {
				t.Fatal("M4A seek differs from sequential PCM", n, err)
			}
			peaks, err := decoder.Peaks(ctx, reader, 40)
			if err != nil || peaks.Frames != info.Frames || len(peaks.Channels) != 2 {
				t.Fatal("M4A waveform", peaks, err)
			}
			// Replace the pathname to prove preview/render still use the retained original.
			if err = os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("replacement is not audio"), 0600); err != nil {
				t.Fatal(err)
			}
			report, err := adapter.PreviewSource(ctx, reader, info, engine.PreviewRequest{Source: path, StartFrame: 12000, EndFrame: 14400, Speed: .75, Pitch: 3})
			if err != nil || report.OutputFrames != 3200 {
				t.Fatal("M4A preview", report, err)
			}
			err = adapter.RenderSource(ctx, reader, info, engine.RenderRequest{Source: path, Destination: filepath.Join(t.TempDir(), "practice.wav"), StartFrame: 12000, EndFrame: 14400, Speed: .75, Pitch: 3})
			if err != nil {
				t.Fatal("M4A render", err)
			}
		})
	}
}
