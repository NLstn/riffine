package native

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/engine"
)

func TestRenderRejectsInvalidRequest(t *testing.T) {
	d := t.TempDir()
	source := filepath.Join(d, "tone.wav")
	filesystem.WriteTone(source, 1, 440, 48000, 1)
	a := Adapter{Executable: "missing-helper"}
	for _, speed := range []float64{0, math.NaN(), 3} {
		err := a.Render(context.Background(), engine.RenderRequest{Source: source, Destination: filepath.Join(d, "out.wav"), EndFrame: 48000, Speed: speed})
		if err == nil {
			t.Fatal("accepted invalid render")
		}
	}
}

// Integration tests are mandatory in native CI; pure-Go users need no compiler.
func TestNativeDSP(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for native integration tests")
	}
	a := Adapter{Executable: helper, Stderr: os.Stderr}
	for _, tc := range []struct {
		name             string
		speed, pitch, hz float64
		channels         int
	}{{"half-speed", .5, 0, 440, 1}, {"octave", 1, 12, 880, 2}, {"combined", .75, -12, 220, 1}, {"double-speed", 2, 0, 440, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			source := filepath.Join(d, "guitär 音.wav")
			dest := filepath.Join(d, "rendér 音.wav")
			if err := filesystem.WriteTone(source, 3, 440, 48000, tc.channels); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			begin := time.Now()
			if err := a.Render(ctx, engine.RenderRequest{Source: source, Destination: dest, StartFrame: 24000, EndFrame: 120000, Speed: tc.speed, Pitch: tc.pitch}); err != nil {
				t.Fatal(err)
			}
			i, err := filesystem.Inspect(context.Background(), dest)
			if err != nil {
				t.Fatal(err)
			}
			if i.Frames != int64(math.Round(96000/tc.speed)) {
				t.Fatal("wrong output duration")
			}
			f, err := os.Open(dest)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			start := int64(float64(i.Frames) * .2)
			frames := int64(float64(i.Frames) * .6)
			b := make([]byte, frames*int64(i.Channels)*2)
			if _, err = f.ReadAt(b, i.DataOffset+start*int64(i.Channels)*2); err != nil {
				t.Fatal(err)
			}
			for c := 0; c < i.Channels; c++ {
				crossings := 0
				var previous int16
				energy := 0.0
				for n := int64(0); n < frames; n++ {
					v := int16(binary.LittleEndian.Uint16(b[(n*int64(i.Channels)+int64(c))*2:]))
					if n > 0 && previous <= 0 && v > 0 {
						crossings++
					}
					previous = v
					energy += math.Pow(float64(v)/32768, 2)
				}
				hz := float64(crossings) * float64(i.SampleRate) / float64(frames)
				rms := math.Sqrt(energy / float64(frames))
				if math.Abs(hz-tc.hz) > tc.hz*.02 || rms < .05 || rms > .5 {
					t.Fatalf("channel %d: hz=%f rms=%f", c, hz, rms)
				}
				t.Logf("channel %d: %.1f Hz, RMS %.3f", c, hz, rms)
			}
			t.Logf("%.3f s output rendered in %s", i.Duration(), time.Since(begin))
			if err = a.Render(ctx, engine.RenderRequest{Source: source, Destination: dest, EndFrame: 144000, Speed: 1}); err == nil {
				t.Fatal("overwrote output")
			}
		})
	}
	t.Run("short-loop", func(t *testing.T) {
		d := t.TempDir()
		src := filepath.Join(d, "short.wav")
		filesystem.WriteTone(src, .01, 440, 48000, 1)
		dest := filepath.Join(d, "out.wav")
		if err := a.Render(context.Background(), engine.RenderRequest{Source: src, Destination: dest, EndFrame: 480, Speed: .25}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		d := t.TempDir()
		src := filepath.Join(d, "src.wav")
		filesystem.WriteTone(src, 1, 440, 48000, 1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		dest := filepath.Join(d, "out.wav")
		if err := a.Render(ctx, engine.RenderRequest{Source: src, Destination: dest, EndFrame: 48000, Speed: 1}); err == nil {
			t.Fatal("ignored cancellation")
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatal("left output on cancellation")
		}
	})
}
