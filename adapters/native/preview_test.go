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
