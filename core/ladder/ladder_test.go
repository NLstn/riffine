package ladder

import (
	"math"
	"testing"
)

func fixture(t *testing.T, end string) *Plan {
	t.Helper()
	p, e := Compile(Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: end})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestRequiredSixtyToOneHundredPercentTrace(t *testing.T) {
	p := fixture(t, "stop")
	if p.Steps() != 9 || p.TotalLoops() != 36 {
		t.Fatal(p.Steps(), p.TotalLoops())
	}
	run, e := Start(p, 1)
	if e != nil {
		t.Fatal(e)
	}
	for ordinal := uint64(1); ordinal <= 36; ordinal++ {
		speed, play := p.SpeedForLoop(ordinal)
		expected := .6 + float64((ordinal-1)/4)*.05
		if !play || math.Abs(speed-expected) > 1e-12 || speed > 1 {
			t.Fatal(ordinal, speed, expected)
		}
		before := run.Snapshot()
		if before.Speed != speed || before.CompletedLoops != ordinal-1 {
			t.Fatal(before)
		}
		accepted, e := run.Complete(1, ordinal)
		if e != nil || !accepted {
			t.Fatal(ordinal, e)
		}
		for _, g := range []uint64{0, 1, 2} {
			accepted, e = run.Complete(g, ordinal)
			if e != nil || accepted {
				t.Fatal("duplicate/stale event advanced run", g, ordinal)
			}
		}
	}
	if s := run.Snapshot(); s.State != "completed" || s.CompletedLoops != 36 || s.Step != 9 || s.Repetitions != 4 || s.Speed != 1 {
		t.Fatal(s)
	}
	if _, play := p.SpeedForLoop(37); play {
		t.Fatal("stop plan continued")
	}
}
func TestHoldTargetAndDecimalStepCount(t *testing.T) {
	p := fixture(t, "hold")
	for _, ordinal := range []uint64{33, 36, 37, 1000, math.MaxUint64} {
		speed, play := p.SpeedForLoop(ordinal)
		if speed != 1 || !play {
			t.Fatal(ordinal, speed, play)
		}
	}
	p, e := Compile(Definition{StartSpeed: .7, TargetSpeed: .8, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"})
	if e != nil || p.Steps() != 3 || p.TotalLoops() != 12 {
		t.Fatal("binary float added phantom target step", p, e)
	}
	p, e = Compile(Definition{StartSpeed: .6, TargetSpeed: .73, IncrementPoints: 5, LoopsPerStep: 2, EndBehavior: "stop"})
	if e != nil {
		t.Fatal(e)
	}
	want := []float64{.6, .6, .65, .65, .7, .7, .73, .73}
	for i, v := range want {
		speed, play := p.SpeedForLoop(uint64(i + 1))
		if speed != v || !play {
			t.Fatal(i, speed, v)
		}
	}
}
func TestEqualTargetAndPauseRestartPolicy(t *testing.T) {
	p, e := Compile(Definition{StartSpeed: .6, TargetSpeed: .6, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"})
	if e != nil || p.Steps() != 1 {
		t.Fatal(e)
	}
	run, e := Start(p, 1)
	if e != nil {
		t.Fatal(e)
	}
	run.Complete(1, 1)
	run.Complete(1, 3)
	run.Pause()
	if ok, _ := run.Complete(1, 4); ok {
		t.Fatal("advanced paused run")
	}
	if e = run.Resume(2); e != nil {
		t.Fatal(e)
	}
	if run.Snapshot().CompletedLoops != 2 {
		t.Fatal("resume reset progress")
	}
	if ok, _ := run.Complete(1, 99); ok {
		t.Fatal("stale generation")
	}
	run.Complete(2, 4)
	run.Complete(2, 5)
	if run.Snapshot().State != "completed" {
		t.Fatal(run.Snapshot())
	}
	if e = run.Restart(3); e != nil {
		t.Fatal(e)
	}
	if s := run.Snapshot(); s.State != "running" || s.CompletedLoops != 0 {
		t.Fatal(s)
	}
	if e = run.Restart(3); e == nil {
		t.Fatal("same generation restarted")
	}
	if e = run.Resume(4); e == nil {
		t.Fatal("unpaused resume")
	}
}
func TestRejectInvalidPlans(t *testing.T) {
	base := fixture(t, "hold").Definition()
	cases := []func(*Definition){func(d *Definition) { d.LoopsPerStep = 0 }, func(d *Definition) { d.LoopsPerStep = MaxLoopsPerStep + 1 }, func(d *Definition) { d.IncrementPoints = 0 }, func(d *Definition) { d.IncrementPoints = -1 }, func(d *Definition) { d.IncrementPoints = math.NaN() }, func(d *Definition) { d.StartSpeed = math.Inf(1) }, func(d *Definition) { d.TargetSpeed = .5 }, func(d *Definition) { d.TargetSpeed = 2.1 }, func(d *Definition) { d.StartSpeed = .24 }, func(d *Definition) { d.EndBehavior = "repeat" }, func(d *Definition) { d.IncrementPoints = 1e-200 }}
	for i, mutate := range cases {
		d := base
		mutate(&d)
		if _, e := Compile(d); e == nil {
			t.Errorf("accepted invalid %d", i)
		}
	}
	if _, play := fixture(t, "hold").SpeedForLoop(0); play {
		t.Fatal("accepted traversal zero")
	}
}
