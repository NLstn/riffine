package native

import (
	"bytes"
	"context"
	"errors"
	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/engine"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenderBorrowedOriginal(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := Adapter{Executable: helper}
	decoder, err := NewDecoder(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, codec := range []string{"wav", "flac", "mp3"} {
		t.Run(codec, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "音-guitär."+codec)
			if codec == "wav" {
				if err := filesystem.WriteTone(path, 1, 440, 48000, 2); err != nil {
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
			request := engine.RenderRequest{Source: path, Destination: filepath.Join(dir, "expected.wav"), StartFrame: 12000, EndFrame: 36000, Speed: .75, Pitch: 3}
			if err = a.Render(ctx, request); err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(request.Destination)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("not audio"), 0600); err != nil {
				t.Fatal(err)
			}
			file, ok := filesystem.BorrowedFile(reader)
			if !ok {
				t.Fatal("original file unavailable")
			}
			if _, err = file.Seek(123, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			request.Destination = filepath.Join(dir, "rendér 音.wav")
			if err = a.RenderSource(ctx, reader, info, request); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(request.Destination)
			if err != nil || !bytes.Equal(expected, actual) {
				t.Fatal("borrowed/path render differs", err)
			}
			if err = a.RenderSource(ctx, reader, info, request); err == nil {
				t.Fatal("overwrote output")
			}
			again, _ := os.ReadFile(request.Destination)
			if !bytes.Equal(actual, again) {
				t.Fatal("changed existing output")
			}
			if pos, err := file.Seek(0, io.SeekCurrent); err != nil || pos != 123 {
				t.Fatal("render changed caller cursor", pos, err)
			}
			after, err := decoder.Inspect(ctx, reader)
			if err != nil || after.SHA256 != info.SHA256 {
				t.Fatal("render changed/closed original", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 4 {
				t.Fatal("render leaked temporary output", entries, err)
			}
		})
	}
}

func TestRenderPublicationRefusesDestinationCreatedDuringProcessing(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.wav")
	dest := filepath.Join(dir, "output.wav")
	if err := filesystem.WriteTone(source, 1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	info, err := filesystem.Inspect(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	request := engine.RenderRequest{Destination: dest, EndFrame: 48000, Speed: 1}
	err = (Adapter{}).renderOutput(context.Background(), info, request, func(tmp string) error {
		if err := filesystem.WriteTone(tmp, 1, 440, 48000, 1); err != nil {
			return err
		}
		return os.WriteFile(dest, []byte("other writer"), 0600)
	})
	if err == nil {
		t.Fatal("replaced concurrent destination")
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "other writer" {
		t.Fatal("changed concurrent destination")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("leaked staging directory")
	}
}

func TestRenderCancellationFencesWorker(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	dir := t.TempDir()
	signal := filepath.Join(dir, "started")
	t.Setenv("RIFFINE_DECODER_TEST_SIGNAL", signal)
	source := filepath.Join(dir, "source.wav")
	if err := filesystem.WriteTone(source, 1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	reader, err := (filesystem.Host{}).Open(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	info, err := filesystem.Inspect(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request := engine.RenderRequest{Destination: filepath.Join(dir, "output.wav"), EndFrame: 48000, Speed: 1}
	done := make(chan error, 1)
	go func() { done <- (Adapter{Executable: os.Args[0]}).RenderSource(ctx, reader, info, request) }()
	for {
		if _, err = os.Stat(signal); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatal("worker ended before cancellation", err)
		case <-ctx.Done():
			t.Fatal("worker never started")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("render did not fence helper")
	}
	if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
		t.Fatal("render closed caller", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("cancelled render published/leaked output", entries)
	}
}

func TestRenderRejectsOlderPreviewHelper(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_DECODER_TEST_LEGACY_PREVIEW", "1")
	a := Adapter{Executable: os.Args[0]}
	if _, err := NewDecoder(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := a.CheckSourceRender(context.Background()); err == nil {
		t.Fatal("advertised unavailable render capability")
	}
}
