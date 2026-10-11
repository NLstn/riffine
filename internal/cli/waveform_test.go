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
