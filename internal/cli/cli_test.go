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

func TestAnnotatedProjectWorkflow(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "tone.wav")
	document := filepath.Join(dir, "project.json")
	annotationFile := filepath.Join(dir, "notes.json")
	saved := filepath.Join(dir, "annotated.json")
	run := func(args ...string) {
		t.Helper()
		var b bytes.Buffer
		if e := Run(context.Background(), args, &b, &b); e != nil {
			t.Fatal(args, e, b.String())
		}
	}
	run("tone", "--out", wav, "--seconds", "1")
	run("import", "--audio", wav, "--out", document)
	original, _ := os.ReadFile(wav)
	previous, _ := os.ReadFile(document)
	if e := os.WriteFile(annotationFile, []byte(`{"notes":"Slow pickup","regions":[{"id":"intro","label":"Intro","start":0.1,"end":0.7}],"loops":[{"id":"repeat","label":"Intro","regionId":"intro"}],"bookmarks":[{"id":"tomorrow","label":"Resume","position":0.3}]}`), 0600); e != nil {
		t.Fatal(e)
	}
	run("annotate", "--project", document, "--out", saved, "--annotations", annotationFile)
	run("inspect", "--project", saved, "--verify")
	service := practice.Service{Resources: filesystem.Host{}, Store: filesystem.Host{}, Decoder: audio.WAVDecoder{}}
	session, e := service.Open(context.Background(), saved)
	if e != nil {
		t.Fatal(e)
	}
	p := session.Snapshot().Project
	if p.Version != 2 || p.Annotations.Notes != "Slow pickup" || len(p.Annotations.Regions) != 1 || p.Annotations.Loops[0].RegionID != "intro" || p.Annotations.Bookmarks[0].Position != .3 {
		t.Fatal(p)
	}
	after, _ := os.ReadFile(wav)
	oldAfter, _ := os.ReadFile(document)
	if !bytes.Equal(after, original) || !bytes.Equal(previous, oldAfter) {
		t.Fatal("annotation changed source or original project")
	}
	var out bytes.Buffer
	if e = Run(context.Background(), []string{"annotate", "--project", document, "--out", saved, "--annotations", annotationFile}, &out, &out); e == nil {
		t.Fatal("overwrote existing annotated project")
	}
}
