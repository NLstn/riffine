// Package ladder evaluates practice speed plans on a serialized control worker.
// It receives authoritative full-traversal completions, never UI timer ticks.
package ladder

import (
	"errors"
	"math"
	"math/big"
	"strconv"
)

const MaxSteps = 10000
const MaxLoopsPerStep = 1000000

type Definition struct {
	StartSpeed      float64 `json:"startSpeed"` // ratio, not percent
	TargetSpeed     float64 `json:"targetSpeed"`
	IncrementPoints float64 `json:"incrementPoints"` // percentage points
	LoopsPerStep    uint32  `json:"loopsPerStep"`
	EndBehavior     string  `json:"endBehavior"` // hold or stop
}

// Plan owns validated immutable values. Decimal ratios use their shortest
// round-trip decimal representation, avoiding phantom steps from binary 0.1.
type Plan struct {
	definition               Definition
	start, target, increment *big.Rat
	steps                    uint64
}

func decimal(v float64) *big.Rat {
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
	return r
}
func Compile(d Definition) (*Plan, error) {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !finite(d.StartSpeed) || !finite(d.TargetSpeed) || !finite(d.IncrementPoints) || d.StartSpeed < .25 || d.TargetSpeed > 2 || d.TargetSpeed < d.StartSpeed || d.IncrementPoints <= 0 || d.LoopsPerStep == 0 || d.LoopsPerStep > MaxLoopsPerStep || (d.EndBehavior != "hold" && d.EndBehavior != "stop") {
		return nil, errors.New("invalid speed ladder definition")
	}
	p := &Plan{definition: d, start: decimal(d.StartSpeed), target: decimal(d.TargetSpeed), increment: new(big.Rat).Quo(decimal(d.IncrementPoints), big.NewRat(100, 1))}
	count := new(big.Rat).Quo(new(big.Rat).Sub(p.target, p.start), p.increment)
	rounded, remainder := new(big.Int), new(big.Int)
	rounded.QuoRem(count.Num(), count.Denom(), remainder)
	if remainder.Sign() != 0 {
		rounded.Add(rounded, big.NewInt(1))
	}
	rounded.Add(rounded, big.NewInt(1))
	if !rounded.IsUint64() || rounded.Uint64() > MaxSteps {
		return nil, errors.New("speed ladder exceeds 10,000 steps")
	}
	p.steps = rounded.Uint64()
	return p, nil
}
func (p *Plan) Definition() Definition { return p.definition }
func (p *Plan) Steps() uint64          { return p.steps }
func (p *Plan) TotalLoops() uint64     { return p.steps * uint64(p.definition.LoopsPerStep) }

// SpeedForLoop uses a one-based eligible traversal ordinal. It can be called
// ahead of playback to compile boundary changes. Stop plans return play=false
// after the configured target repetitions; hold plans keep playing at target.
func (p *Plan) SpeedForLoop(ordinal uint64) (speed float64, play bool) {
	if ordinal == 0 {
		return 0, false
	}
	if p.definition.EndBehavior == "stop" && ordinal > p.TotalLoops() {
		return p.definition.TargetSpeed, false
	}
	step := (ordinal - 1) / uint64(p.definition.LoopsPerStep)
	if step >= p.steps-1 {
		return p.definition.TargetSpeed, true
	}
	value := new(big.Rat).Add(p.start, new(big.Rat).Mul(p.increment, new(big.Rat).SetInt(new(big.Int).SetUint64(step))))
	speed, _ = value.Float64()
	return min(speed, p.definition.TargetSpeed), true
}

type Snapshot struct {
	State          string  `json:"state"` // running, paused or completed
	Generation     uint64  `json:"generation"`
	CompletedLoops uint64  `json:"completedLoops"`
	Step           uint64  `json:"step"`        // one-based current step
	Repetitions    uint64  `json:"repetitions"` // completed in current step
	Speed          float64 `json:"speed"`
	NextSpeed      float64 `json:"nextSpeed"`
}
type Run struct {
	plan                                *Plan
	generation, lastSequence, completed uint64
	state                               string
}

func Start(p *Plan, generation uint64) (*Run, error) {
	if p == nil || generation == 0 {
		return nil, errors.New("plan and generation are required")
	}
	return &Run{plan: p, generation: generation, state: "running"}, nil
}
func (r *Run) Snapshot() Snapshot {
	next := r.completed + 1
	if r.completed == math.MaxUint64 {
		next = r.completed
	}
	speed, play := r.plan.SpeedForLoop(next)
	step := r.completed/uint64(r.plan.definition.LoopsPerStep) + 1
	step = min(step, r.plan.steps)
	repetitions := r.completed - (step-1)*uint64(r.plan.definition.LoopsPerStep)
	repetitions = min(repetitions, uint64(r.plan.definition.LoopsPerStep))
	nextSpeed, _ := r.plan.SpeedForLoop(min(step*uint64(r.plan.definition.LoopsPerStep), math.MaxUint64-1) + 1)
	state := r.state
	if !play {
		state = "completed"
	}
	return Snapshot{State: state, Generation: r.generation, CompletedLoops: r.completed, Step: step, Repetitions: repetitions, Speed: speed, NextSpeed: nextSpeed}
}

// Complete accepts an eligible, fully presented traversal from the transport.
// Partial completions must never call this method. Schedule sequence can skip
// ineligible partial traversals; duplicate/stale events do not advance progress.
func (r *Run) Complete(generation, sequence uint64) (bool, error) {
	if r.state != "running" || generation != r.generation || sequence == 0 || sequence <= r.lastSequence {
		return false, nil
	}
	if r.completed == math.MaxUint64 {
		return false, errors.New("ladder loop counter exhausted")
	}
	r.lastSequence = sequence
	r.completed++
	if r.plan.definition.EndBehavior == "stop" && r.completed == r.plan.TotalLoops() {
		r.state = "completed"
	}
	return true, nil
}
func (r *Run) Pause() {
	if r.state == "running" {
		r.state = "paused"
	}
}

// Resume preserves repetitions but invalidates old-generation acknowledgements.
// Transport suspend/reset must precede it. Seek, loop edit and explicit restart
// instead call Restart. A manual speed override pauses the run until explicit resume.
func (r *Run) Resume(generation uint64) error {
	if r.state != "paused" || generation <= r.generation {
		return errors.New("resume requires a paused run and newer generation")
	}
	r.generation = generation
	r.lastSequence = 0
	r.state = "running"
	return nil
}
func (r *Run) Restart(generation uint64) error {
	if generation <= r.generation {
		return errors.New("restart requires a newer generation")
	}
	r.generation = generation
	r.lastSequence = 0
	r.completed = 0
	r.state = "running"
	return nil
}

// Rebind preserves progress/state across a transport reset (for example pitch
// changes or suspension) while invalidating every previous acknowledgement.
func (r *Run) Rebind(generation uint64) error {
	if generation <= r.generation {
		return errors.New("rebind requires a newer generation")
	}
	r.generation = generation
	r.lastSequence = 0
	return nil
}

func (r *Run) Plan() *Plan { return r.plan }

type Step struct {
	Index     uint64  `json:"index"`
	FirstLoop uint64  `json:"firstLoop"`
	LastLoop  uint64  `json:"lastLoop"`
	Speed     float64 `json:"speed"`
}

func (p *Plan) Describe() []Step {
	steps := make([]Step, p.steps)
	for i := range steps {
		first := uint64(i)*uint64(p.definition.LoopsPerStep) + 1
		speed, _ := p.SpeedForLoop(first)
		steps[i] = Step{Index: uint64(i) + 1, FirstLoop: first, LastLoop: first + uint64(p.definition.LoopsPerStep) - 1, Speed: speed}
	}
	return steps
}
