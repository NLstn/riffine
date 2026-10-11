package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/waveform"
)

func TestWaveformWindowCacheAndBounds(t *testing.T) {
	dir := t.TempDir()
	tone := filepath.Join(dir, "tone.wav")
	project := filepath.Join(dir, "project.json")
	cache := filepath.Join(dir, "cache")
	var out, errOut bytes.Buffer
	ctx := context.Background()
	for _, args := range [][]string{{"tone", "--out", tone, "--seconds", ".2"}, {"import", "--audio", tone, "--out", project}} {
		if err := Run(ctx, args, &out, &errOut); err != nil {
			t.Fatal(err)
		}
	}
	var first waveform.Window
	for n, want := range []string{"rebuilt", "hit"} {
		out.Reset()
		args := []string{"waveform-window", "--project", project, "--cache", cache, "--start-frame", "255", "--end-frame", "8193", "--bins", "3"}
		if err := Run(ctx, args, &out, &errOut); err != nil {
			t.Fatal(err)
		}
		var report struct {
			CacheStatus string
			waveform.Window
		}
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.CacheStatus != want || len(report.Cells) > 3 || report.RequestedStart != 255 || report.RequestedEnd != 8193 {
			t.Fatal(report)
		}
		if n == 0 {
			first = report.Window
		} else if !reflect.DeepEqual(first, report.Window) {
			t.Fatal("cache changed waveform")
		}
	}
	for _, args := range [][]string{{"--bins", "0"}, {"--start-frame", "-1"}, {"--end-frame", "9601"}, {"--end-frame", "0"}} {
		if err := Run(ctx, append([]string{"waveform-window", "--project", project}, args...), &out, &errOut); err == nil {
			t.Fatal("bad window accepted", args)
		}
	}
}

func TestWaveformDetailExactCLISelection(t *testing.T) {
	dir := t.TempDir()
	tone, project := filepath.Join(dir, "tone.wav"), filepath.Join(dir, "project.json")
	ctx := context.Background()
	var out, errOut bytes.Buffer
	for _, args := range [][]string{{"tone", "--out", tone, "--seconds", ".2"}, {"import", "--audio", tone, "--out", project}, {"waveform-detail", "--project", project, "--start-frame", "255", "--end-frame", "258", "--bins", "4096"}} {
		out.Reset()
		if err := Run(ctx, args, &out, &errOut); err != nil {
			t.Fatal(err)
		}
	}
	var window waveform.Window
	if err := json.Unmarshal(out.Bytes(), &window); err != nil || window.FramesPerCell != 1 || len(window.Cells) != 3 || window.Cells[0].StartFrame != 255 || window.Cells[2].EndFrame != 258 {
		t.Fatal(window, err)
	}
	for _, args := range [][]string{{"--start-frame", "-1"}, {"--end-frame", "9601"}, {"--end-frame", "0"}, {"--bins", "0"}, {"--bins", "4097"}, {"--cache", filepath.Join(dir, "cache")}} {
		if err := Run(ctx, append([]string{"waveform-detail", "--project", project}, args...), &out, &errOut); err == nil {
			t.Fatal("invalid detail accepted", args)
		}
	}
}
