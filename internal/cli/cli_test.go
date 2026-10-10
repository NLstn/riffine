package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/NLstn/riffine/core/engine"
	"os"
	"path/filepath"
	"testing"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/practice"
)

func TestPracticeWorkflow(t *testing.T) {
	d := t.TempDir()
	wav := filepath.Join(d, "tone.wav")
	p := filepath.Join(d, "p.json")
	slow := filepath.Join(d, "slow.json")
	loop := filepath.Join(d, "loop.json")
	run := func(args ...string) {
		t.Helper()
		var out bytes.Buffer
		if err := Run(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("%v: %v (%s)", args, err, out.String())
		}
	}
	run("tone", "--out", wav, "--seconds", "1")
	run("import", "--audio", wav, "--out", p)
	run("set", "--project", p, "--out", slow, "--speed", "0.5", "--pitch", "-2")
	run("loop", "--project", slow, "--out", loop, "--start", "0.1", "--end", "0.8")
	run("inspect", "--project", loop, "--verify")
	run("waveform", "--project", loop, "--bins", "20", "--out", filepath.Join(d, "peaks.json"))
	service := practice.Service{Resources: filesystem.Host{}, Store: filesystem.Host{}, Decoder: audio.WAVDecoder{}}
	session, err := service.Open(context.Background(), loop)
	got := session.Snapshot().Project
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Speed != .5 || got.Settings.Pitch != -2 || !got.Settings.Loop.Enabled {
		t.Fatalf("bad settings: %+v", got.Settings)
	}
	disabled := filepath.Join(d, "disabled.json")
	run("loop", "--project", loop, "--out", disabled, "--enabled=false")
	session, err = service.Open(context.Background(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	got = session.Snapshot().Project
	if got.Settings.Loop.Enabled || got.Settings.Loop.Start != .1 {
		t.Fatal("disable changed boundaries")
	}
	var out bytes.Buffer
	if err = Run(context.Background(), []string{"set", "--project", p, "--out", slow, "--speed", "1"}, &out, &out); err == nil {
		t.Fatal("overwrote existing output")
	}
}

func TestBadArguments(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"import"}, {"tone", "unexpected"}, {"set", "--speed", ".5"}, {"render", "--project", "missing"}} {
		var out bytes.Buffer
		if err := Run(context.Background(), args, &out, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestSilentStreamingPreviewCLI(t *testing.T) {
	if os.Getenv("RIFFINE_AUDIO_HELPER") == "" {
		t.Skip("native helper required for null-device CLI integration")
	}
	d := t.TempDir()
	source := filepath.Join(d, "tone.wav")
	document := filepath.Join(d, "song.json")
	var output bytes.Buffer
	for _, args := range [][]string{{"tone", "--out", source, "--seconds", ".1"}, {"import", "--audio", source, "--out", document}, {"preview", "--project", document, "--silent"}} {
		output.Reset()
		if err := Run(context.Background(), args, &output, os.Stderr); err != nil {
			t.Fatal(err)
		}
	}
	var report engine.PreviewReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Mode != "null" || report.CopiedFrames != 4800 {
		t.Fatalf("CLI preview failed: %+v", report)
	}
	entries, err := os.ReadDir(d)
	if err != nil || len(entries) != 2 {
		t.Fatal("CLI preview created a rendered WAV")
	}
}
