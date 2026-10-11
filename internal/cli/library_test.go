package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/library"
)

func TestScanLocalSources(t *testing.T) {
	dir := t.TempDir()
	tone := filepath.Join(dir, "音楽.wav")
	if err := filesystem.WriteTone(tone, .01, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.mp3"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "sources.json")
	if err := os.WriteFile(list, []byte(`["音楽.wav","音楽.wav","bad.mp3","missing.wav"]`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), []string{"scan", "--sources", list}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Version int
		Formats []string
		library.Batch
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Version != 1 || !report.Done || len(report.Formats) != 1 || report.Formats[0] != "wav-pcm" || len(report.Results) != 4 {
		t.Fatal(report)
	}
	for i, want := range []string{"indexed", "duplicate", "unavailable", "unavailable"} {
		if report.Results[i].Status != want {
			t.Fatal(report.Results)
		}
	}
	if report.Results[0].Track.Title != "音楽" {
		t.Fatal(report)
	}
	for _, input := range []string{"[]", "{}", `["x"] {}`, strings.Repeat(" ", 1<<20+1)} {
		if err := os.WriteFile(list, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Run(context.Background(), []string{"scan", "--sources", list}, &out, &errOut); err == nil {
			t.Fatal("bad source list accepted")
		}
	}
}
