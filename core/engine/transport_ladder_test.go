package engine

import (
	"context"
	"testing"

	"github.com/NLstn/riffine/core/ladder"
)

func setPlan(t *testing.T, tr *Transport, end string) {
	t.Helper()
	d := ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: end}
	command(t, tr, "set_plan", func(c *TransportCommand) { c.Plan = &d })
}
func TestTransportPlansAudibleBoundarySpeedsAhead(t *testing.T) {
	tr, f := setupTransport(t)
	setPlan(t, tr, "stop")
	command(t, tr, "play", nil)
	for i := 0; i < 36; i++ {
		s := f.schedules[i]
		want, _ := tr.planRun.Plan().SpeedForLoop(uint64(i + 1))
		if s.Speed != want || s.Pitch != 3 {
			t.Fatal(i+1, s, want)
		}
		f.acks = []OutputAcknowledgement{consumed(s, s.OutputEnd)}
		state := poll(t, tr)
		if state.Plan.CompletedLoops != uint64(i+1) {
			t.Fatal(state)
		}
		events := tr.DrainEvents()
		if len(events) != 1 || events[0].Speed != s.Speed || events[0].Pitch != 3 {
			t.Fatal(events)
		}
	}
	final := tr.Snapshot()
	if f.running || final.State != "stopped" || final.Plan.State != "completed" || final.CompletedLoops != 36 || len(f.schedules) != 36 {
		t.Fatal(final, len(f.schedules))
	}
	if _, e := tr.Dispatch(context.Background(), TransportCommand{Version: 1, Sequence: final.Sequence + 1, Type: "play"}); e == nil {
		t.Fatal("replayed completed stop plan without restart")
	}
	command(t, tr, "restart", nil)
	if tr.Snapshot().Plan.CompletedLoops != 0 || f.schedules[36].Speed != .6 {
		t.Fatal(tr.Snapshot())
	}
}
func TestTransportLadderPausePartialOverrideAndSeek(t *testing.T) {
	tr, f := setupTransport(t)
	setPlan(t, tr, "hold")
	command(t, tr, "play", nil)
	first := f.schedules[0]
	f.acks = []OutputAcknowledgement{consumed(first, first.OutputEnd)}
	poll(t, tr)
	tr.DrainEvents()
	current := f.schedules[1]
	f.acks = []OutputAcknowledgement{consumed(current, current.OutputStart+(current.OutputEnd-current.OutputStart)/2)}
	paused := command(t, tr, "pause", nil)
	if paused.Plan.CompletedLoops != 1 || paused.Plan.State != "paused" {
		t.Fatal(paused)
	}
	command(t, tr, "play", nil)
	partial := f.schedules[len(f.schedules)-2]
	f.acks = []OutputAcknowledgement{consumed(partial, partial.OutputEnd)}
	if s := poll(t, tr); s.Plan.CompletedLoops != 1 {
		t.Fatal("resumed partial advanced plan", s)
	}
	manual := 1.2
	overridden := command(t, tr, "set_parameters", func(c *TransportCommand) { c.Speed = &manual })
	if overridden.Plan.State != "paused" || f.schedules[len(f.schedules)-2].Speed != 1.2 {
		t.Fatal(overridden)
	}
	command(t, tr, "pause", nil)
	command(t, tr, "play", nil)
	if tr.Snapshot().Plan.State != "paused" {
		t.Fatal("transport resume undid manual override")
	}
	command(t, tr, "resume_plan", nil)
	if s := tr.Snapshot(); s.Plan.State != "running" || s.Plan.CompletedLoops != 1 || s.Config.Speed != .6 {
		t.Fatal(s)
	}
	position := int64(24000)
	seeked := command(t, tr, "seek", func(c *TransportCommand) { c.Position = &position })
	if seeked.Plan.CompletedLoops != 0 || seeked.SourceFrame != position {
		t.Fatal(seeked)
	}
	partial = f.schedules[len(f.schedules)-2]
	f.acks = []OutputAcknowledgement{consumed(partial, partial.OutputEnd)}
	if s := poll(t, tr); s.Plan.CompletedLoops != 0 {
		t.Fatal("seek partial counted", s)
	}
	command(t, tr, "set_loop", func(c *TransportCommand) { c.Loop = &TransportLoop{} })
	if tr.Snapshot().Plan.State != "paused" {
		t.Fatal("disabled loop advanced ladder")
	}
	command(t, tr, "stop", nil)
	command(t, tr, "play", nil)
	if tr.Snapshot().Plan.State != "paused" {
		t.Fatal("playing whole track resumed a loop ladder")
	}
	command(t, tr, "clear_plan", nil)
	if tr.Snapshot().Plan != nil {
		t.Fatal("plan retained after clear")
	}
}

func TestClearingLadderAtPendingBoundaryPreservesAudibleSpeed(t *testing.T) {
	tr, f := setupTransport(t)
	setPlan(t, tr, "hold")
	command(t, tr, "play", nil)
	for i := 0; i < 3; i++ {
		s := f.schedules[i]
		f.acks = []OutputAcknowledgement{consumed(s, s.OutputEnd)}
		poll(t, tr)
		tr.DrainEvents()
	}
	fourth := f.schedules[3]
	f.acks = []OutputAcknowledgement{consumed(fourth, fourth.OutputEnd)}
	cleared := command(t, tr, "clear_plan", nil)
	if cleared.Plan != nil || cleared.Config.Speed != .65 || f.schedules[len(f.schedules)-2].Speed != .65 || cleared.CompletedLoops != 4 {
		t.Fatal(cleared)
	}
}

func TestParameterEditAtFinalPendingBoundaryCannotRestartCompletedPlan(t *testing.T) {
	tr, f := setupTransport(t)
	setPlan(t, tr, "stop")
	command(t, tr, "play", nil)
	for i := 0; i < 35; i++ {
		s := f.schedules[i]
		f.acks = []OutputAcknowledgement{consumed(s, s.OutputEnd)}
		poll(t, tr)
		tr.DrainEvents()
	}
	last := f.schedules[35]
	f.acks = []OutputAcknowledgement{consumed(last, last.OutputEnd)}
	pitch := -2.0
	final := command(t, tr, "set_parameters", func(c *TransportCommand) { c.Pitch = &pitch })
	if final.State != "stopped" || final.Plan.State != "completed" || f.running || len(f.schedules) != 36 {
		t.Fatal("edit restarted completed ladder", final)
	}
}
