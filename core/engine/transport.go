package engine

import (
	"context"
	"errors"
	"github.com/NLstn/riffine/core/ladder"
	"math"
	"sync"
)

// TransportDevice runs on a control worker. Suspend must stop consumption and
// return all acknowledgements through that exact boundary before returning.
// Reset leaves playback suspended. Start begins consuming previously scheduled
// output. Acknowledgements refer to presented device-rate frames, with DSP and
// device buffering excluded; callback submission alone cannot attest playback.
// Implementations must preserve ordering and report bounded-queue overflow.
// Every failed device operation must leave output suspended. No operation may
// re-enter Transport while its serialized control call is in progress.
type TransportDevice interface {
	OutputDevice
	Start(context.Context) error
	Suspend(context.Context) ([]OutputAcknowledgement, error)
}

const TransportVersion = 1
const transportEventCapacity = 128

// TransportConfig uses source frames, speed ratios and pitch semitones. The
// output rate may differ from the source rate. Metadata has already been verified
// by the session; this controller does not open or decode resources.
type TransportConfig struct {
	Source      string  `json:"source"`
	Frames      int64   `json:"frames"`
	SampleRate  int     `json:"sampleRate"`
	OutputRate  int     `json:"outputRate"`
	Speed       float64 `json:"speed"`
	Pitch       float64 `json:"pitch"`
	LoopEnabled bool    `json:"loopEnabled"`
	LoopStart   int64   `json:"loopStart"`
	LoopEnd     int64   `json:"loopEnd"`
}
type TransportCommand struct {
	Version  int                `json:"version"`
	Sequence uint64             `json:"sequence"`
	Type     string             `json:"type"`
	Position *int64             `json:"position,omitempty"`
	Speed    *float64           `json:"speed,omitempty"`
	Pitch    *float64           `json:"pitch,omitempty"`
	Loop     *TransportLoop     `json:"loop,omitempty"`
	Plan     *ladder.Definition `json:"plan,omitempty"`
}
type TransportLoop struct {
	Enabled    bool  `json:"enabled"`
	StartFrame int64 `json:"startFrame"`
	EndFrame   int64 `json:"endFrame"`
}
type TransportSnapshot struct {
	Version             int              `json:"version"`
	Sequence            uint64           `json:"sequence"`
	Generation          uint64           `json:"generation"`
	State               string           `json:"state"`
	SourceFrame         int64            `json:"sourceFrame"`
	ConsumedOutputFrame int64            `json:"consumedOutputFrame"`
	CompletedLoops      uint64           `json:"completedLoops"`
	Config              TransportConfig  `json:"config"`
	Error               string           `json:"error,omitempty"`
	Plan                *ladder.Snapshot `json:"plan,omitempty"`
}
type TransportEvent struct {
	Type           string  `json:"type"`
	Generation     uint64  `json:"generation"`
	Sequence       uint64  `json:"sequence"` // schedule sequence, independent of commands
	CompletedLoops uint64  `json:"completedLoops"`
	StartFrame     int64   `json:"startFrame"`
	EndFrame       int64   `json:"endFrame"`
	Speed          float64 `json:"speed"`
	Pitch          float64 `json:"pitch"`
}
type plannedTraversal struct {
	schedule Schedule
	gate     *Traversal
	eligible bool
}

// Transport serializes commands, adapter acknowledgements and snapshots. It
// plans at most two traversals ahead. Hosts pump Poll on a control worker and
// render snapshots; neither UI timers nor audio callbacks decide loop policy.
// Clock is deliberately absent: only acknowledged consumption advances position.
type Transport struct {
	mu               sync.Mutex
	device           TransportDevice
	snapshot         TransportSnapshot
	plans            []plannedTraversal
	resumeEligible   bool
	planRun          *ladder.Run
	autoResumePlan   bool
	scheduleSequence uint64
	events           []TransportEvent
}

func NewTransport(device TransportDevice) (*Transport, error) {
	if device == nil {
		return nil, errors.New("transport device is required")
	}
	return &Transport{device: device, snapshot: TransportSnapshot{Version: TransportVersion, State: "loading"}}, nil
}
func validateTransport(c TransportConfig) error {
	if c.Source == "" || c.Frames <= 0 || c.Frames > 1<<48 || c.SampleRate < 8000 || c.SampleRate > 192000 || c.OutputRate < 8000 || c.OutputRate > 192000 || math.IsNaN(c.Speed) || math.IsInf(c.Speed, 0) || c.Speed < .25 || c.Speed > 2 || math.IsNaN(c.Pitch) || math.IsInf(c.Pitch, 0) || c.Pitch < -12 || c.Pitch > 12 {
		return errors.New("invalid transport configuration")
	}
	if c.LoopStart < 0 || c.LoopEnd < c.LoopStart || c.LoopEnd > c.Frames || (c.LoopEnabled && c.LoopEnd == c.LoopStart) {
		return errors.New("invalid transport loop")
	}
	return nil
}
func (t *Transport) Snapshot() TransportSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.currentSnapshot()
}
func (t *Transport) currentSnapshot() TransportSnapshot {
	s := t.snapshot
	if t.planRun != nil {
		progress := t.planRun.Snapshot()
		s.Plan = &progress
	}
	return s
}
func (t *Transport) fail(err error) error {
	t.snapshot.State = "error"
	t.snapshot.Error = err.Error()
	return err
}
func (t *Transport) failedSnapshot(err error) (TransportSnapshot, error) {
	t.fail(err)
	return t.currentSnapshot(), err
}
func (t *Transport) bounds() (int64, int64) {
	c := t.snapshot.Config
	if c.LoopEnabled {
		return c.LoopStart, c.LoopEnd
	}
	return 0, c.Frames
}
func (t *Transport) reset(ctx context.Context) error {
	if t.snapshot.Generation == math.MaxUint64 {
		return t.fail(errors.New("transport generation exhausted"))
	}
	t.snapshot.Generation++
	t.plans = nil
	t.snapshot.ConsumedOutputFrame = 0
	if err := t.device.Reset(ctx, t.snapshot.Generation); err != nil {
		return t.fail(err)
	}
	return nil
}

// Load is an explicit resource boundary and resets command sequence and loop count.
// Validation failures leave the previous session intact. Device failures are
// terminal until recover/load, because a host may have partially applied work.
func (t *Transport) Load(ctx context.Context, c TransportConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateTransport(c); err != nil {
		return err
	}
	if t.snapshot.State == "playing" {
		if err := t.suspend(ctx); err != nil {
			return err
		}
	}
	if err := t.reset(ctx); err != nil {
		return err
	}
	t.snapshot.Config = c
	t.snapshot.Sequence = 0
	t.snapshot.State = "ready"
	t.snapshot.Error = ""
	t.snapshot.CompletedLoops = 0
	t.snapshot.SourceFrame, _ = t.bounds()
	t.events = nil
	t.resumeEligible = true
	t.planRun = nil
	return nil
}
func (t *Transport) plan(ctx context.Context, start int64, eligible bool) error {
	_, end := t.bounds()
	if start >= end {
		return nil
	}
	if t.scheduleSequence == math.MaxUint64 {
		return t.fail(errors.New("schedule sequence exhausted"))
	}
	outputStart := t.snapshot.ConsumedOutputFrame
	if len(t.plans) > 0 {
		outputStart = t.plans[len(t.plans)-1].schedule.OutputEnd
	}
	c := t.snapshot.Config
	speed := c.Speed
	if t.planRun != nil && t.planRun.Snapshot().State == "running" {
		ordinal := t.planRun.Snapshot().CompletedLoops + 1
		for _, p := range t.plans {
			if p.eligible {
				ordinal++
			}
		}
		var play bool
		speed, play = t.planRun.Plan().SpeedForLoop(ordinal)
		if !play {
			return nil
		}
	}
	frames := int64(math.Round(float64(end-start) * float64(c.OutputRate) / float64(c.SampleRate) / speed))
	if frames < 1 {
		frames = 1
	}
	if outputStart > math.MaxInt64-frames {
		return t.fail(errors.New("output timeline exhausted"))
	}
	t.scheduleSequence++
	s := Schedule{Generation: t.snapshot.Generation, Sequence: t.scheduleSequence, Source: c.Source, StartFrame: start, EndFrame: end, OutputStart: outputStart, OutputEnd: outputStart + frames, Speed: speed, Pitch: c.Pitch}
	s.EdgeFadeFrames = min(int64(c.OutputRate/200), (frames+1)/2)
	gate, err := NewTraversal(s)
	if err != nil {
		return t.fail(err)
	}
	if err = t.device.Schedule(ctx, s); err != nil {
		return t.fail(err)
	}
	t.plans = append(t.plans, plannedTraversal{s, gate, eligible})
	return nil
}
func (t *Transport) fill(ctx context.Context, eligible bool) error {
	if len(t.plans) == 0 {
		if err := t.plan(ctx, t.snapshot.SourceFrame, eligible); err != nil {
			return err
		}
	}
	if t.snapshot.Config.LoopEnabled {
		start, _ := t.bounds()
		for len(t.plans) < 2 {
			before := len(t.plans)
			if err := t.plan(ctx, start, true); err != nil {
				return err
			}
			if len(t.plans) == before {
				break
			}
		}
	}
	return nil
}
func (t *Transport) acknowledge(acks []OutputAcknowledgement) error {
	if len(acks) > transportEventCapacity {
		return t.fail(errors.New("adapter acknowledgement batch exceeds bound"))
	}
	for _, a := range acks {
		if a.Generation != t.snapshot.Generation {
			continue
		}
		if len(t.plans) == 0 {
			if a.Sequence > t.scheduleSequence {
				return t.fail(errors.New("acknowledgement references unscheduled output"))
			}
			continue
		}
		p := t.plans[0]
		s := p.schedule
		if a.Sequence < s.Sequence {
			continue
		}
		if a.Sequence != s.Sequence {
			return t.fail(errors.New("out-of-order output acknowledgement"))
		}
		complete, err := p.gate.Acknowledge(a)
		if err != nil {
			return t.fail(err)
		}
		t.snapshot.Config.Speed = s.Speed
		t.snapshot.ConsumedOutputFrame = a.ConsumedOutputFrame
		t.resumeEligible = p.eligible && a.ConsumedOutputFrame == s.OutputStart
		// Interpolate using this schedule's exact rounded output length, including
		// source/device rate conversion. Never add decode-ahead or wall-clock time.
		fraction := float64(a.ConsumedOutputFrame-s.OutputStart) / float64(s.OutputEnd-s.OutputStart)
		t.snapshot.SourceFrame = s.StartFrame + int64(math.Floor(fraction*float64(s.EndFrame-s.StartFrame)))
		if complete != nil {
			if p.eligible {
				if len(t.events) == transportEventCapacity {
					return t.fail(errors.New("transport event queue overflow; drain events before polling"))
				}
				if t.snapshot.Config.LoopEnabled {
					if t.snapshot.CompletedLoops == math.MaxUint64 {
						return t.fail(errors.New("loop counter exhausted"))
					}
					t.snapshot.CompletedLoops++
					if t.planRun != nil {
						if _, err := t.planRun.Complete(s.Generation, s.Sequence); err != nil {
							return t.fail(err)
						}
					}
				}
				t.events = append(t.events, TransportEvent{Type: "traversal_completed", Generation: s.Generation, Sequence: s.Sequence, CompletedLoops: t.snapshot.CompletedLoops, StartFrame: s.StartFrame, EndFrame: s.EndFrame, Speed: s.Speed, Pitch: s.Pitch})
			}
			t.plans = t.plans[1:]
			if t.snapshot.Config.LoopEnabled {
				t.snapshot.SourceFrame, _ = t.bounds()
				t.resumeEligible = true
			}
		}
	}
	if len(t.plans) > 0 {
		t.snapshot.Config.Speed = t.plans[0].schedule.Speed
	}
	return nil
}
func (t *Transport) suspend(ctx context.Context) error {
	acks, err := t.device.Suspend(ctx)
	if err != nil {
		return t.fail(err)
	}
	return t.acknowledge(acks)
}

// Poll consumes a bounded batch and replenishes planned traversals. A host must
// report starvation instead of repeating audio without a scheduled sequence.
func (t *Transport) Poll(ctx context.Context) (TransportSnapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return t.currentSnapshot(), err
	}
	if t.snapshot.State != "playing" {
		return t.currentSnapshot(), nil
	}
	acks, err := t.device.DrainAcknowledgements(ctx, transportEventCapacity)
	if err != nil {
		return t.failedSnapshot(err)
	}
	if err = t.acknowledge(acks); err != nil {
		_, stopErr := t.device.Suspend(ctx)
		return t.currentSnapshot(), errors.Join(err, stopErr)
	}
	if len(t.plans) == 0 && (!t.snapshot.Config.LoopEnabled || t.planRun != nil && t.planRun.Snapshot().State == "completed") {
		if err = t.suspend(ctx); err != nil {
			return t.currentSnapshot(), err
		}
		t.snapshot.State = "stopped"
		return t.currentSnapshot(), nil
	}
	if len(t.plans) > 0 {
		t.snapshot.Config.Speed = t.plans[0].schedule.Speed
	}
	err = t.fill(ctx, true)
	if err != nil {
		_, stopErr := t.device.Suspend(ctx)
		return t.currentSnapshot(), errors.Join(err, stopErr)
	}
	return t.currentSnapshot(), nil
}
func (t *Transport) DrainEvents() []TransportEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	events := t.events
	t.events = nil
	return events
}

// Dispatch uses contiguous successful command sequences. Invalid or cancelled
// commands have no effect; adapter failures enter error and do not consume the
// command sequence. Recovery always resets the generation and resumes stopped.
func (t *Transport) Dispatch(ctx context.Context, c TransportCommand) (TransportSnapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	reject := func(message string) (TransportSnapshot, error) { return t.currentSnapshot(), errors.New(message) }
	if err := ctx.Err(); err != nil {
		return t.currentSnapshot(), err
	}
	if c.Version != TransportVersion || c.Sequence == 0 || t.snapshot.Sequence == math.MaxUint64 || c.Sequence != t.snapshot.Sequence+1 {
		return reject("invalid transport command version or sequence")
	}
	if t.snapshot.State == "loading" {
		return reject("load a source before transport commands")
	}
	if t.snapshot.State == "error" && c.Type != "recover" {
		return reject("transport requires recovery")
	}
	next := t.snapshot.Config
	supplied := 0
	if c.Position != nil {
		supplied++
	}
	if c.Speed != nil {
		supplied++
	}
	if c.Pitch != nil {
		supplied++
	}
	if c.Loop != nil {
		supplied++
	}
	if c.Plan != nil {
		supplied++
	}
	switch c.Type {
	case "play", "pause", "stop", "restart", "recover", "resume_plan", "clear_plan":
		if supplied != 0 {
			return reject("unexpected transport command fields")
		}
	case "seek":
		if supplied != 1 || c.Position == nil || *c.Position < 0 || *c.Position >= next.Frames {
			return reject("invalid seek")
		}
		a, b := t.bounds()
		if *c.Position < a || *c.Position >= b {
			return reject("seek must remain within the active selection")
		}
	case "set_parameters":
		if c.Plan != nil || c.Position != nil || c.Loop != nil || supplied == 0 {
			return reject("parameters require speed or pitch")
		}
		if c.Speed != nil {
			next.Speed = *c.Speed
		}
		if c.Pitch != nil {
			next.Pitch = *c.Pitch
		}
	case "set_loop":
		if supplied != 1 || c.Loop == nil {
			return reject("loop command requires loop only")
		}
		next.LoopEnabled = c.Loop.Enabled
		next.LoopStart = c.Loop.StartFrame
		next.LoopEnd = c.Loop.EndFrame
	case "set_plan":
		if supplied != 1 || c.Plan == nil || !next.LoopEnabled {
			return reject("set_plan requires an enabled loop and plan only")
		}
		if _, err := ladder.Compile(*c.Plan); err != nil {
			return t.currentSnapshot(), err
		}
	default:
		return reject("unsupported transport command")
	}
	if err := validateTransport(next); err != nil {
		return t.currentSnapshot(), err
	}
	if c.Type == "recover" && t.snapshot.State != "error" {
		return reject("recovery requires an error")
	}
	if c.Type == "play" && t.snapshot.State == "playing" || c.Type == "pause" && t.snapshot.State != "playing" {
		return reject("command is invalid in the current transport state")
	}
	if c.Type == "resume_plan" && (t.planRun == nil || t.planRun.Snapshot().State != "paused" || !next.LoopEnabled) {
		return reject("resume_plan requires a paused plan and enabled loop")
	}
	if c.Type == "play" && t.planRun != nil && t.planRun.Snapshot().State == "completed" {
		return reject("completed plan requires restart or clear_plan")
	}
	wasPlaying := t.snapshot.State == "playing"
	if wasPlaying {
		if err := t.suspend(ctx); err != nil {
			return t.currentSnapshot(), err
		}
	}
	if t.planRun != nil && t.planRun.Snapshot().State == "completed" && c.Type == "set_parameters" {
		wasPlaying = false
		t.snapshot.State = "stopped"
	}
	if c.Speed == nil {
		next.Speed = t.snapshot.Config.Speed
	}
	if err := t.reset(ctx); err != nil {
		return t.currentSnapshot(), err
	}
	t.snapshot.Config = next
	start, end := t.bounds()
	switch c.Type {
	case "pause":
		t.snapshot.State = "paused"
	case "stop", "recover":
		t.snapshot.State = "stopped"
		t.snapshot.SourceFrame = start
		t.resumeEligible = true
		t.snapshot.CompletedLoops = 0
		t.snapshot.Error = ""
	case "restart":
		t.snapshot.SourceFrame = start
		t.resumeEligible = true
		t.snapshot.CompletedLoops = 0
		t.snapshot.State = "playing"
	case "play":
		if t.snapshot.SourceFrame >= end {
			t.snapshot.SourceFrame = start
			t.resumeEligible = true
		}
		t.snapshot.State = "playing"
	case "seek":
		t.snapshot.SourceFrame = *c.Position
		t.resumeEligible = *c.Position == start
		t.snapshot.CompletedLoops = 0
	case "set_loop":
		t.snapshot.SourceFrame = start
		t.resumeEligible = true
		t.snapshot.CompletedLoops = 0
	}
	if c.Type == "seek" || c.Type == "set_loop" || c.Type == "set_parameters" {
		if wasPlaying {
			t.snapshot.State = "playing"
		}
	}
	if c.Type == "clear_plan" {
		t.planRun = nil
	}
	if c.Type == "set_plan" {
		plan, _ := ladder.Compile(*c.Plan)
		t.planRun, _ = ladder.Start(plan, t.snapshot.Generation)
		t.snapshot.SourceFrame = start
		t.resumeEligible = true
		t.snapshot.CompletedLoops = 0
		t.snapshot.Config.Speed = plan.Definition().StartSpeed
		t.autoResumePlan = true
		if !wasPlaying {
			t.planRun.Pause()
		}
	} else if t.planRun != nil {
		generation := t.snapshot.Generation
		switch c.Type {
		case "restart", "seek", "set_loop", "stop", "recover":
			if err := t.planRun.Restart(generation); err != nil {
				return t.failedSnapshot(err)
			}
			t.snapshot.Config.Speed = t.planRun.Plan().Definition().StartSpeed
			t.autoResumePlan = true
			if t.snapshot.State != "playing" || !t.snapshot.Config.LoopEnabled {
				t.planRun.Pause()
			}
		case "pause":
			t.planRun.Pause()
			if err := t.planRun.Rebind(generation); err != nil {
				return t.failedSnapshot(err)
			}
		case "play", "resume_plan":
			if c.Type == "resume_plan" || t.autoResumePlan && t.snapshot.Config.LoopEnabled && t.planRun.Snapshot().State == "paused" {
				if err := t.planRun.Resume(generation); err != nil {
					return t.failedSnapshot(err)
				}
				t.snapshot.Config.Speed = t.planRun.Snapshot().Speed
				t.autoResumePlan = true
			} else {
				if err := t.planRun.Rebind(generation); err != nil {
					return t.failedSnapshot(err)
				}
			}
		default:
			if c.Type == "set_parameters" && c.Speed != nil {
				t.planRun.Pause()
				t.autoResumePlan = false
			}
			if err := t.planRun.Rebind(generation); err != nil {
				return t.failedSnapshot(err)
			}
		}
	}
	if t.snapshot.State == "playing" {
		// Any interrupted partial traversal remains ineligible after resume/edit.
		eligible := t.resumeEligible && t.snapshot.SourceFrame == start
		if err := t.fill(ctx, eligible); err != nil {
			return t.currentSnapshot(), err
		}
		if err := t.device.Start(ctx); err != nil {
			return t.failedSnapshot(err)
		}
	}
	t.snapshot.Sequence = c.Sequence
	return t.currentSnapshot(), nil
}
