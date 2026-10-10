package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/NLstn/riffine/core/project"
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
	got, err := project.Load(loop)
	if err != nil {
		t.Fatal(err)
	}
	if got.Settings.Speed != .5 || got.Settings.Pitch != -2 || !got.Settings.Loop.Enabled {
		t.Fatalf("bad settings: %+v", got.Settings)
	}
	disabled := filepath.Join(d, "disabled.json")
	run("loop", "--project", loop, "--out", disabled, "--enabled=false")
	got, _ = project.Load(disabled)
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
