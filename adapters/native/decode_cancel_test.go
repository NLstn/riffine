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
			if os.Getenv("RIFFINE_DECODER_TEST_LEGACY_SOURCE") == "1" {
				fmt.Print(`{"abi":1,"formats":["wav-pcm","flac","mp3"]}`)
			} else {
				fmt.Print(`{"abi":1,"formats":["wav-pcm","flac","mp3"],"borrowedSources":true}`)
			}
		case "decoder-info-handle":
			frames := 48000
			if os.Getenv("RIFFINE_DECODER_TEST_FINAL_WAIT") == "1" {
				frames = 1024
			}
			fmt.Printf(`{"encoding":"mp3","sampleRate":48000,"channels":1,"bits":0,"frames":%d}`, frames)
		case "decoder-read-handle":
			if os.Getenv("RIFFINE_DECODER_TEST_FINAL_WAIT") == "1" {
				if _, err := os.Stdout.Write(make([]byte, 1024*4)); err != nil {
					os.Exit(1)
				}
			}
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
	testStalledWorkerCancellation(t, false)
}
func TestFinalReadCancellationKillsStalledWorker(t *testing.T) {
	testStalledWorkerCancellation(t, true)
}
func testStalledWorkerCancellation(t *testing.T, finalWait bool) {
	if finalWait {
		t.Setenv("RIFFINE_DECODER_TEST_FINAL_WAIT", "1")
	}
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

func TestDecoderRejectsHelperWithoutBorrowedSourceCapability(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_DECODER_TEST_LEGACY_SOURCE", "1")
	if _, err := NewDecoder(context.Background(), Adapter{Executable: os.Args[0]}); err == nil {
		t.Fatal("advertised compressed decoding with an incompatible helper")
	}
}
