package engine

import (
	"context"
	"github.com/NLstn/riffine/core/timing"
	"slices"
	"time"
)

// Clock returns monotonic host elapsed time. It is never a source-frame clock.
type Clock interface{ Now() time.Duration }

// Schedule is a bounded control-worker message for future live adapters. The
// source range is [StartFrame, EndFrame). OutputStart/End are device-rate frames
// consumed by the output device, excluding algorithm latency and queued output.
// No callback may call core or choose practice policy. All fields are explicit
// so an adapter can compile work into preallocated realtime schedules.
type Schedule struct {
	Generation             uint64
	Sequence               uint64
	Source                 string
	StartFrame, EndFrame   int64
	OutputStart, OutputEnd int64
	Speed, Pitch           float64
	// EdgeFadeFrames requests independent linear gain ramps at both ends.
	// The adapter does not overlap, shorten, or expand the source selection.
	EdgeFadeFrames  int64
	Phase           string           `json:"phase,omitempty"` // empty=audio; count_in/rest never complete a loop
	CueOffsetFrames int64            `json:"cueOffsetFrames,omitempty"`
	Clicks          []timing.Click   `json:"clicks,omitempty"` // finite count-in, original phase coordinates
	ClickLevel      float64          `json:"clickLevel,omitempty"`
	BeatGrid        *timing.Grid     `json:"-"`
	BeatSelection   timing.Selection `json:"-"`
}
type OutputAcknowledgement struct {
	Generation          uint64
	Sequence            uint64
	ConsumedOutputFrame int64
}

// OutputDevice is a provisional control port for #14/#15, not an implemented
// capability. DrainAcknowledgements must be bounded, nonblocking and ordered.
// Overflow is an explicit error; traversal acknowledgements must not be dropped.
// Reset invalidates all prior schedules/acknowledgements by generation.
type OutputDevice interface {
	Schedule(context.Context, Schedule) error
	Reset(context.Context, uint64) error
	DrainAcknowledgements(context.Context, int) ([]OutputAcknowledgement, error)
}

// Optional capabilities prevent hosts from silently playing cue-enabled plans
// as plain loops. These methods run on a control worker, never an audio callback.
type CueDevice interface{ SupportsCues() bool }
type MetronomeDevice interface{ SupportsMetronome() bool }

func (s Schedule) Audio() bool { return s.Phase == "" }
func (s Schedule) ClickWindow(ctx context.Context, start, end int64) ([]timing.Click, error) {
	if s.BeatGrid == nil {
		return nil, nil
	}
	batch, err := s.BeatGrid.Clicks(ctx, s.BeatSelection, start, end)
	return batch.Clicks, err
}

func (s Schedule) Clone() Schedule { s.Clicks = slices.Clone(s.Clicks); return s }
