package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/NLstn/riffine/core/timing"
)

var ErrUnsupportedCues = errors.New("cue phases are unavailable")
var ErrUnsupportedMetronome = errors.New("metronome is unavailable")

func (t *Transport) cueHost(c TransportConfig) error {
	if !c.Cues.Enabled() {
		return nil
	}
	device, ok := t.device.(CueDevice)
	if !ok || !device.SupportsCues() {
		return fmt.Errorf("%w: this output host does not support count-in/rest phases", ErrUnsupportedCues)
	}
	if c.Cues.Metronome {
		device, ok := t.device.(MetronomeDevice)
		if !ok || !device.SupportsMetronome() {
			return fmt.Errorf("%w: this output host does not support the metronome", ErrUnsupportedMetronome)
		}
	}
	return nil
}
func (t *Transport) nextSpeed() (float64, bool) {
	speed := t.snapshot.Config.Speed
	if t.planRun != nil && t.planRun.Snapshot().State == "running" {
		ordinal := t.planRun.Snapshot().CompletedLoops + 1
		for _, p := range t.plans {
			if p.schedule.Audio() && p.eligible {
				ordinal++
			}
		}
		return t.planRun.Plan().SpeedForLoop(ordinal)
	}
	return speed, true
}
func (t *Transport) outputStart() int64 {
	if len(t.plans) > 0 {
		return t.plans[len(t.plans)-1].schedule.OutputEnd
	}
	return t.snapshot.ConsumedOutputFrame
}
func (t *Transport) cue(ctx context.Context, phase string, anchor int64, resume *Schedule) error {
	c := t.snapshot.Config
	speed, play := t.nextSpeed()
	if !play {
		return nil
	}
	s := Schedule{Source: c.Source, StartFrame: anchor, EndFrame: anchor, Phase: phase, Speed: speed, Pitch: c.Pitch}
	var frames int64
	if resume != nil {
		s = *resume
		frames = s.OutputEnd - s.OutputStart
	} else if phase == "rest" {
		frames = c.Cues.RestFrames(c.OutputRate)
	} else {
		selection := timing.Selection{StartFrame: anchor, EndFrame: c.Frames, OutputRate: c.OutputRate, Speed: speed}
		if c.LoopEnabled {
			selection.EndFrame = c.LoopEnd
		}
		batch, count, err := t.grid.CountIn(ctx, selection, c.Cues.CountInBeats)
		if err != nil {
			return t.fail(err)
		}
		s.Clicks = batch.Clicks
		s.ClickLevel = c.Cues.ClickLevel
		frames = count
	}
	if frames < 1 || t.scheduleSequence == math.MaxUint64 || t.outputStart() > math.MaxInt64-frames {
		return t.fail(errors.New("cue output timeline exhausted"))
	}
	t.scheduleSequence++
	s.Generation = t.snapshot.Generation
	s.Sequence = t.scheduleSequence
	s.OutputStart = t.outputStart()
	s.OutputEnd = s.OutputStart + frames
	gate, err := NewTraversal(s)
	if err != nil {
		return t.fail(err)
	}
	if err = t.device.Schedule(ctx, s.Clone()); err != nil {
		return t.fail(err)
	}
	t.plans = append(t.plans, plannedTraversal{s, gate, false})
	return nil
}

// Preserve exact remaining device frames and original click phase when output
// is suspended. Cue progress is presentation-driven, never elapsed wall time.
func (t *Transport) preserveCue() {
	if len(t.plans) == 0 || t.plans[0].schedule.Audio() {
		return
	}
	s := t.plans[0].schedule
	consumed := max(s.OutputStart, t.snapshot.ConsumedOutputFrame)
	s.CueOffsetFrames += consumed - s.OutputStart
	s.OutputEnd -= consumed
	s.OutputStart = 0
	s.Clicks = slices.Clone(s.Clicks)
	t.resumeCue = &s
}
func (t *Transport) phaseSnapshot(s *TransportSnapshot) {
	var current *Schedule
	if len(t.plans) > 0 {
		current = &t.plans[0].schedule
	} else if t.resumeCue != nil {
		current = t.resumeCue
	}
	if current != nil {
		if current.Audio() {
			s.Phase = "audio"
		} else {
			s.Phase = current.Phase
		}
		s.PhaseRemainingFrames = current.OutputEnd - max(current.OutputStart, s.ConsumedOutputFrame)
	}
}
