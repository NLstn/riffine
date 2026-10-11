package native

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NLstn/riffine/adapters/filesystem"
)

// The test executable provides a controlled stalled worker, never an external
// shell script. This verifies cancellation can interrupt a blocked PCM pipe.
func TestMain(m *testing.M) {
	if os.Getenv("RIFFINE_DECODER_TEST_PROCESS") == "1" {
		switch os.Args[1] {
		case "formats":
			fmt.Print(`{"abi":1,"formats":["wav-pcm","flac","mp3"]}`)
		case "decoder-info":
			fmt.Print(`{"encoding":"mp3","sampleRate":48000,"channels":1,"bits":0,"frames":48000}`)
		case "decoder-read":
			if err := os.WriteFile(os.Getenv("RIFFINE_DECODER_TEST_SIGNAL"), []byte("started"), 0600); err != nil {
				os.Exit(1)
			}
			time.Sleep(time.Hour)
		default:
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestReadCancellationKillsStalledWorker(t *testing.T) {
	dir := t.TempDir()
	signal := filepath.Join(dir, "started")
	path := filepath.Join(dir, "fake-source.mp3")
	if err := os.WriteFile(path, []byte("controlled fake source"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_DECODER_TEST_SIGNAL", signal)
	ctx, deadline := context.WithTimeout(context.Background(), 10*time.Second)
	defer deadline()
	decoder, err := NewDecoder(ctx, Adapter{Executable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	r, err := (filesystem.Host{}).Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	stream, err := decoder.OpenPCM(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := stream.ReadFrames(readCtx, make([]float32, 1024)); done <- err }()
	for {
		if _, err = os.Stat(signal); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatal("worker exited before cancellation", err)
		case <-ctx.Done():
			t.Fatal("worker did not start")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("incorrect cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked PCM read ignored cancellation")
	}
	if err = stream.Close(); err != nil {
		t.Fatal(err)
	}
	// The original borrowed file is still available after decoder teardown.
	if _, err = r.ReadAt(make([]byte, 4), 0); err != nil {
		t.Fatal("decoder closed original reader", err)
	}
}
