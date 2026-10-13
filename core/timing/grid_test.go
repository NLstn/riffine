package timing

import (
	"context"
	"math"
	"reflect"
	"testing"
)

func manual(t *testing.T, bpm float64) *Grid {
	t.Helper()
	g, err := Compile(Definition{Version: 1, SourceBPM: bpm, BeatsPerBar: 4}, 48000, 48000*1800)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestOriginalTempoScalesWithSpeedAndDeviceRate(t *testing.T) {
	g := manual(t, 120)
	ctx := context.Background()
	for _, rate := range []int{44100, 48000, 96000} {
		for _, speed := range []float64{.25, .6, 1, 2} {
			s := Selection{StartFrame: 0, EndFrame: 48000 * 4, OutputRate: rate, Speed: speed}
			total, _ := g.OutputFrames(s)
			batch, err := g.Clicks(ctx, s, 0, total)
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Clicks) != 8 {
				t.Fatal(batch)
			}
			for i, click := range batch.Clicks {
				want := int64(math.Round(float64(i) * .5 * float64(rate) / speed))
				if click.OutputFrame != want || click.Accent != (i%4 == 0) {
					t.Fatal(rate, speed, click, want)
				}
			}
			count, frames, err := g.CountIn(ctx, s, 4)
			if err != nil || frames != int64(math.Round(2*float64(rate)/speed)) || len(count.Clicks) != 4 {
				t.Fatal(count, frames, err)
			}
		}
	}
}
func TestWindowPartitionHasNoDriftOrDuplicateBoundaryClicks(t *testing.T) {
	g := manual(t, 137)
	s := Selection{StartFrame: 479, EndFrame: 48000 * 1800, OutputRate: 44100, Speed: .6}
	total, _ := g.OutputFrames(s)
	var previous int64 = -1
	var ordinal int64
	for start := int64(0); start < total; start += 4096 {
		batch, err := g.Clicks(context.Background(), s, start, min(total, start+4096))
		if err != nil {
			t.Fatal(err)
		}
		for _, click := range batch.Clicks {
			if click.OutputFrame <= previous || click.Ordinal != ordinal+1 {
				t.Fatal("duplicate or missing pulse", click, ordinal)
			}
			want := g.outputFrame(s, float64(click.Ordinal)*60*48000/137)
			if click.OutputFrame != want {
				t.Fatal("drift", click, want)
			}
			previous = click.OutputFrame
			ordinal = click.Ordinal
		}
	}
	if ordinal < 4000 {
		t.Fatal("long timeline was not exercised", ordinal)
	}
}
func TestExplicitVariableMapAndOwnership(t *testing.T) {
	definition := Definition{Version: 1, BeatsPerBar: 3, Markers: []Beat{{0, 0}, {24000, 1}, {43200, 2}, {72000, 3}, {96000, 4}}}
	g, err := Compile(definition, 48000, 144000)
	if err != nil {
		t.Fatal(err)
	}
	definition.Markers[0].SourceFrame = 500
	copy := g.Definition()
	copy.Markers[0].SourceFrame = 600
	s := Selection{StartFrame: 12000, EndFrame: 96000, OutputRate: 44100, Speed: .5}
	total, _ := g.OutputFrames(s)
	batch, err := g.Clicks(context.Background(), s, 0, total)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Click{{22050, 1, false}, {57330, 2, false}, {110250, 3, true}}
	if !reflect.DeepEqual(batch.Clicks, expected) {
		t.Fatal(batch)
	}
	count, frames, err := g.CountIn(context.Background(), s, 3)
	if err != nil || frames != 132300 || count.Clicks[1].OutputFrame != 44100 {
		t.Fatal(count, frames, err)
	}
}
func TestInvalidBoundedAndCancelledQueries(t *testing.T) {
	for _, d := range []Definition{{Version: 2, SourceBPM: 120, BeatsPerBar: 4}, {Version: 1, SourceBPM: math.NaN(), BeatsPerBar: 4}, {Version: 1, SourceBPM: 120, BeatsPerBar: 0}, {Version: 1, BeatsPerBar: 4, Markers: []Beat{{100, 0}, {99, 1}}}, {Version: 1, SourceBPM: 120, BeatsPerBar: 4, Markers: []Beat{{100, 0}}}} {
		if _, err := Compile(d, 48000, 480000); err == nil {
			t.Fatal(d)
		}
	}
	g := manual(t, 400)
	s := Selection{0, 48000 * 1800, 48000, .25}
	total, _ := g.OutputFrames(s)
	if batch, err := g.Clicks(context.Background(), s, 0, total); err == nil || len(batch.Clicks) != 0 {
		t.Fatal("unbounded or partial result", batch, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Clicks(ctx, s, 0, 4096); err != context.Canceled {
		t.Fatal(err)
	}
	if _, _, err := g.CountIn(ctx, s, 4); err != context.Canceled {
		t.Fatal(err)
	}
	if _, _, err := g.CountIn(context.Background(), s, 33); err == nil {
		t.Fatal("unbounded count-in")
	}
	mapGrid, _ := Compile(Definition{Version: 1, BeatsPerBar: 4, Markers: []Beat{{0, 0}, {24000, 2}}}, 48000, 480000)
	if _, _, err := mapGrid.CountIn(context.Background(), Selection{0, 48000, 48000, 1}, 4); err == nil {
		t.Fatal("missing beat tempo was guessed")
	}
}

func TestEndMarkerDefinesTempoWithoutClickAtExclusiveEnd(t *testing.T) {
	g, err := Compile(Definition{Version: 1, BeatsPerBar: 4, Markers: []Beat{{0, 0}, {48000, 1}}}, 48000, 48000)
	if err != nil {
		t.Fatal(err)
	}
	selection := Selection{0, 48000, 48000, 1}
	batch, err := g.Clicks(context.Background(), selection, 0, 48000)
	if err != nil || len(batch.Clicks) != 1 || batch.Clicks[0].OutputFrame != 0 {
		t.Fatal(batch, err)
	}
	_, frames, err := g.CountIn(context.Background(), selection, 2)
	if err != nil || frames != 96000 {
		t.Fatal(frames, err)
	}
}
