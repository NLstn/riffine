package native

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/engine"
)

func TestBoundedPreviewReport(t *testing.T) {
	var report boundedReport
	n, err := io.Copy(&report, bytes.NewReader(bytes.Repeat([]byte("x"), 16384)))
	if err != nil || n != 16384 || report.Len() != 8192 || !report.overflow {
		t.Fatal("unbounded report capture")
	}
	_, _ = report.Write([]byte("more"))
	if report.Len() != 8192 {
		t.Fatal("report grew after overflow")
	}
}
func TestPreviewCancelledBeforeHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Adapter{Executable: "missing"}).Preview(ctx, engine.PreviewRequest{Source: "source", EndFrame: 48000, Speed: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestNativePreview(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	d := t.TempDir()
	source := filepath.Join(d, "guitär 音.wav")
	if err := filesystem.WriteTone(source, 3, 440, 48000, 2); err != nil {
		t.Fatal(err)
	}
	before, err := filesystem.Inspect(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	a := Adapter{Executable: helper, Stderr: os.Stderr, SilentPreview: true}
	for _, speed := range []float64{.25, 1, 2} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		report, err := a.Preview(ctx, engine.PreviewRequest{Source: source, StartFrame: 12000, EndFrame: 14400, Speed: speed, Pitch: 3})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if report.Channels != 2 || report.Backend != "Null" || report.CopiedFrames != int64(2400/speed) {
			t.Fatalf("bad report: %+v", report)
		}
		t.Logf("null-device preview: %+v", report)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	began := time.Now()
	_, err = a.Preview(ctx, engine.PreviewRequest{Source: source, EndFrame: 144000, Speed: .25})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(began) > 2*time.Second {
		t.Fatalf("in-flight cancellation: %v after %s", err, time.Since(began))
	}
	if _, err = a.Preview(context.Background(), engine.PreviewRequest{Source: source, EndFrame: 999999, Speed: 1}); err == nil {
		t.Fatal("accepted source range overflow")
	}
	after, err := filesystem.Inspect(context.Background(), source)
	if err != nil || before.SHA256 != after.SHA256 {
		t.Fatal("preview changed original source")
	}
	entries, err := os.ReadDir(d)
	if err != nil || len(entries) != 1 {
		t.Fatal("preview created a temporary render")
	}
}

func TestPreviewRejectsHelperWithoutBorrowedPreview(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_DECODER_TEST_LEGACY_PREVIEW", "1")
	a := Adapter{Executable: os.Args[0]}
	if _, err := NewDecoder(context.Background(), a); err != nil {
		t.Fatal("older decoder should remain usable", err)
	}
	if err := a.CheckSourcePreview(context.Background()); err == nil {
		t.Fatal("advertised unavailable source preview")
	}
}

func TestPreviewBorrowedOriginal(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := Adapter{Executable: helper, SilentPreview: true}
	decoder, err := NewDecoder(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, codec := range []string{"wav", "flac", "mp3"} {
		t.Run(codec, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "guitär 音."+codec)
			if codec == "wav" {
				if err := filesystem.WriteTone(path, 3, 440, 48000, 2); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(filepath.Join("../../native/testdata", "generated-stereo."+codec))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			reader, err := (filesystem.Host{}).Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			info, err := decoder.Inspect(ctx, reader)
			if err != nil {
				t.Fatal(err)
			}
			file, ok := filesystem.BorrowedFile(reader)
			if !ok {
				t.Fatal("missing original resource")
			}
			if _, err = file.Seek(123, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("replacement is not audio"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, speed := range []float64{.25, 1, 2} {
				request := engine.PreviewRequest{Source: path, StartFrame: 12000, EndFrame: 14400, Speed: speed, Pitch: 3}
				report, err := a.PreviewSource(ctx, reader, info, request)
				if err != nil || report.OutputFrames != int64(2400/speed) || report.Channels != info.Channels || report.SampleRate != info.SampleRate {
					t.Fatal(report, err)
				}
			}
			if pos, err := file.Seek(0, io.SeekCurrent); err != nil || pos != 123 {
				t.Fatal("preview changed caller cursor", pos, err)
			}
			cancelled, stop := context.WithCancel(ctx)
			stop()
			if _, err = a.PreviewSource(cancelled, reader, info, engine.PreviewRequest{}); !errors.Is(err, context.Canceled) {
				t.Fatal("pre-cancelled preview", err)
			}
			if _, err = a.PreviewSource(ctx, reader, info, engine.PreviewRequest{EndFrame: info.Frames + 1, Speed: 1}); err == nil {
				t.Fatal("accepted source overflow")
			}
			short, stop := context.WithTimeout(ctx, 100*time.Millisecond)
			_, err = a.PreviewSource(short, reader, info, engine.PreviewRequest{EndFrame: info.Frames, Speed: .25})
			stop()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("preview cancellation did not fence helper", err)
			}
			if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
				t.Fatal("preview closed original", err)
			}
			after, err := decoder.Inspect(ctx, reader)
			if err != nil || after.SHA256 != info.SHA256 {
				t.Fatal("original resource changed", after, err)
			}
		})
	}
}
