package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
)

type recorder struct{ request engine.RenderRequest }

func (r *recorder) Render(_ context.Context, q engine.RenderRequest) error { r.request = q; return nil }
func service(r engine.Renderer) practice.Service {
	return practice.Service{Resources: Host{}, Store: Host{}, Decoder: audio.WAVDecoder{}, Renderer: r}
}

func TestSharedConformanceAndSaveAs(t *testing.T) {
	var fixture struct {
		Audio struct {
			Name                 string
			Seconds, Hz          float64
			SampleRate, Channels int
		}
		Steps []struct {
			Command      practice.Command
			Error        practice.ErrorCode
			Sequence     uint64
			Speed, Pitch float64
			Loop         project.Loop
		}
		Events []practice.Event
		Render struct {
			StartFrame, EndFrame int64
			Speed, Pitch         float64
		}
	}
	b, err := os.ReadFile("../../core/practice/testdata/session-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	d := t.TempDir()
	source := filepath.Join(d, fixture.Audio.Name)
	document := filepath.Join(d, "project.json")
	a := fixture.Audio
	if err = WriteTone(source, a.Seconds, a.Hz, a.SampleRate, a.Channels); err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	s, err := service(r).Import(context.Background(), source, document)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range fixture.Steps {
		got, err := s.Dispatch(context.Background(), step.Command)
		var apiError *practice.Error
		if step.Error == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.As(err, &apiError) || apiError.Code != step.Error {
			t.Fatalf("error=%v, want %s", err, step.Error)
		}
		if got.Sequence != step.Sequence || got.Project.Settings != (project.Settings{Speed: step.Speed, Pitch: step.Pitch, Loop: step.Loop}) {
			t.Fatalf("snapshot mismatch: %+v", got)
		}
	}
	if got := s.DrainEvents(); got.Dropped != 0 || !reflect.DeepEqual(got.Events, fixture.Events) {
		t.Fatalf("event mismatch: %+v", got)
	}
	if err = s.Render(context.Background(), filepath.Join(d, "render.wav")); err != nil {
		t.Fatal(err)
	}
	if r.request.StartFrame != fixture.Render.StartFrame || r.request.EndFrame != fixture.Render.EndFrame || r.request.Speed != fixture.Render.Speed || r.request.Pitch != fixture.Render.Pitch {
		t.Fatalf("render mismatch: %+v", r.request)
	}
	if err = s.SaveAs(context.Background(), document); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(document)
	if err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(d, "elsewhere")
	if err = os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(subdir, "saved.json")
	if err = s.SaveAs(context.Background(), destination); err != nil {
		t.Fatal(err)
	}
	opened, err := service(r).Open(context.Background(), destination)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Snapshot().Project.Settings != s.Snapshot().Project.Settings {
		t.Fatal("lost settings during save-as")
	}
	if err = opened.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = opened.SaveAs(context.Background(), document); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrote existing project: %v", err)
	}
	after, err := os.ReadFile(document)
	if err != nil || string(after) != string(original) {
		t.Fatal("prior project changed")
	}
	renamed := filepath.Join(d, "moved 音.wav")
	if err = os.Rename(source, renamed); err != nil {
		t.Fatal(err)
	}
	if err = opened.Relink(context.Background(), renamed); err != nil {
		t.Fatal(err)
	}
	if err = opened.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestCreateExclusiveAndCancelled(t *testing.T) {
	h := Host{}
	d := t.TempDir()
	path := filepath.Join(d, "project.json")
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- h.Create(context.Background(), path, []byte("complete")) }()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("exclusive creation had %d winners", winners)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "complete" {
		t.Fatal("incomplete publication")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = h.Create(ctx, filepath.Join(d, "cancelled.json"), []byte("data")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err = h.Read(context.Background(), path, 2); err == nil {
		t.Fatal("ignored document limit")
	}
	entries, err := os.ReadDir(d)
	if err != nil || len(entries) != 1 {
		t.Fatalf("left staging files: %v, %v", entries, err)
	}
}
func TestToneCollisionAndInvalidInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := WriteTone(path, 1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	if err := WriteTone(path, 1, 220, 48000, 1); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrote source: %v", err)
	}
	i, err := Inspect(context.Background(), path)
	if err != nil || i.Frames != 48000 {
		t.Fatalf("inspect: %+v, %v", i, err)
	}
	invalid := filepath.Join(t.TempDir(), "invalid.wav")
	if err := WriteTone(invalid, 0, 440, 48000, 1); err == nil {
		t.Fatal("accepted invalid tone")
	}
	if _, err = os.Stat(invalid); !os.IsNotExist(err) {
		t.Fatal("published invalid tone")
	}
}

func TestBorrowedResourceRetainsOriginalAcrossReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "original.bin")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := (Host{}).Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	borrowed, ok := BorrowedFile(r)
	if !ok {
		t.Fatal("did not lend owned file")
	}
	if err = os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 8)
	if _, err = borrowed.ReadAt(data, 0); err != nil || string(data) != "original" {
		t.Fatal("borrowed resource followed replacement", string(data), err)
	}
	if _, ok = BorrowedFile(nil); ok {
		t.Fatal("lent a nonexistent resource")
	}
}
