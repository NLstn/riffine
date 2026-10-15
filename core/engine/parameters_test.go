package engine

import (
	"context"
	"errors"
	"github.com/NLstn/riffine/core/ladder"
	"testing"
)

type parameterFake struct {
	transportFake
	change ParameterChange
	err    error
	calls  int
}

func (f *parameterFake) ConfigureParameters(_ context.Context, sequence uint64, speed, pitch float64) (ParameterChange, error) {
	f.calls++
	return f.change, f.err
}

func parameterTransport(t *testing.T) (*Transport, *parameterFake) {
	t.Helper()
	d := &parameterFake{}
	tr, err := NewTransport(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = tr.Load(context.Background(), TransportConfig{Source: "fixture", Frames: 96000, SampleRate: 48000, OutputRate: 48000, Speed: 1, LoopEnabled: true, LoopEnd: 96000}); err != nil {
		t.Fatal(err)
	}
	command(t, tr, "play", nil)
	return tr, d
}

func TestContinuousParametersPreservePresentationAndDiscardFuture(t *testing.T) {
	tr, d := parameterTransport(t)
	before := tr.Snapshot()
	generations := len(d.generations)
	first := d.schedules[0]
	d.change = ParameterChange{Applied: true, Sequence: first.Sequence, SourceFrame: 12000, OutputFrame: 12000, OutputEnd: 180000, Acknowledgements: []OutputAcknowledgement{{Generation: before.Generation, Sequence: first.Sequence, ConsumedOutputFrame: 4000}}}
	speed, pitch := .5, 3.
	edited := command(t, tr, "set_parameters", func(c *TransportCommand) { c.Speed = &speed; c.Pitch = &pitch })
	if !d.running || len(d.generations) != generations || edited.Generation != before.Generation || edited.SourceFrame != 4000 || edited.ConsumedOutputFrame != 4000 {
		t.Fatal("edit stopped output or advanced presentation", edited)
	}
	if len(tr.plans) != 3 || tr.plans[1].schedule.Sequence != 3 || tr.plans[1].schedule.Speed != .5 || tr.plans[1].schedule.Pitch != 3 {
		t.Fatal("future schedules were not replaced", tr.plans)
	}
	d.acks = []OutputAcknowledgement{{Generation: before.Generation, Sequence: first.Sequence, ConsumedOutputFrame: 12000}, {Generation: before.Generation, Sequence: 3, ConsumedOutputFrame: 24000}}
	s, err := tr.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.SourceFrame != 18000 || s.CompletedLoops != 0 || s.Config.Speed != .5 {
		t.Fatal("wrong piecewise presentation", s)
	}
	if len(d.schedules) != 3 || d.schedules[2].Speed != .5 || d.schedules[2].OutputStart != 180000 {
		t.Fatal("continued loop has stale parameters", d.schedules)
	}
	d.acks = []OutputAcknowledgement{{Generation: before.Generation, Sequence: 3, ConsumedOutputFrame: 180000}, {Generation: before.Generation, Sequence: 4, ConsumedOutputFrame: 372000}}
	s, err = tr.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.CompletedLoops != 1 {
		t.Fatal("interrupted partial counted as a loop", s)
	}
	events := tr.DrainEvents()
	if len(events) != 1 || events[0].Sequence != 4 || events[0].Speed != .5 {
		t.Fatal(events)
	}
}

func TestContinuousParameterDeclineRetainsFallback(t *testing.T) {
	tr, d := parameterTransport(t)
	before := tr.Snapshot()
	speed := .75
	s := command(t, tr, "set_parameters", func(c *TransportCommand) { c.Speed = &speed })
	if s.Generation != before.Generation+1 || !d.running || s.Config.Speed != speed {
		t.Fatal(s)
	}
}

func TestContinuousParameterFailureQuiesces(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		tr, d := parameterTransport(t)
		before := tr.Snapshot()
		speed := .75
		if invalid {
			d.change = ParameterChange{Applied: true, Sequence: 1, SourceFrame: 12000, OutputFrame: 12000, OutputEnd: 1}
		} else {
			d.err = errors.New("DSP failed")
		}
		s, err := tr.Dispatch(context.Background(), TransportCommand{Version: 1, Sequence: before.Sequence + 1, Type: "set_parameters", Speed: &speed})
		if err == nil || s.State != "error" || d.running || s.Sequence != before.Sequence {
			t.Fatal(s, err)
		}
	}
}

func TestContinuousManualSpeedPausesPlan(t *testing.T) {
	tr, d := parameterTransport(t)
	command(t, tr, "set_plan", func(c *TransportCommand) {
		c.Plan = &ladder.Definition{StartSpeed: 1, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 2, EndBehavior: "hold"}
	})
	before := tr.Snapshot()
	first := tr.plans[0].schedule
	d.change = ParameterChange{Applied: true, Sequence: first.Sequence, SourceFrame: 12000, OutputFrame: 12000, OutputEnd: 180000}
	speed := .5
	s := command(t, tr, "set_parameters", func(c *TransportCommand) { c.Speed = &speed })
	if s.Generation != before.Generation || s.Plan.State != "paused" || s.Config.Speed != .5 || tr.autoResumePlan {
		t.Fatal(s)
	}
	command(t, tr, "pause", nil)
	s = command(t, tr, "play", nil)
	if s.Plan.State != "paused" || s.Config.Speed != .5 {
		t.Fatal("pause/play lost manual override", s)
	}
}
