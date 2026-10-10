package engine

import (
	"errors"
	"math"
)

// Traversal is a single scheduled traversal acknowledgement gate shared by the
// feasibility hosts. It is not a transport state machine or a routine evaluator.
// Only the adapter can attest that acknowledged output was actually consumed.
// The live engine must reset/drop this gate on seeks/edits/interruption (#15).
type Traversal struct {
	schedule  Schedule
	consumed  int64
	completed bool
}
type TraversalCompletion struct {
	Type                string `json:"type"`
	Generation          uint64 `json:"generation"`
	Sequence            uint64 `json:"sequence"`
	StartFrame          int64  `json:"startFrame"`
	EndFrame            int64  `json:"endFrame"`
	ConsumedOutputFrame int64  `json:"consumedOutputFrame"`
}

func NewTraversal(s Schedule) (*Traversal, error) {
	if s.Generation == 0 || s.Sequence == 0 || s.Source == "" || s.StartFrame < 0 || s.EndFrame <= s.StartFrame || s.OutputStart < 0 || s.OutputEnd <= s.OutputStart || math.IsNaN(s.Speed) || math.IsInf(s.Speed, 0) || s.Speed < .25 || s.Speed > 2 || math.IsNaN(s.Pitch) || math.IsInf(s.Pitch, 0) || s.Pitch < -12 || s.Pitch > 12 {
		return nil, errors.New("invalid traversal schedule")
	}
	return &Traversal{schedule: s, consumed: s.OutputStart}, nil
}

// Acknowledge is called by an ordered control worker, never a device callback.
// ConsumedOutputFrame is an exclusive end position. Stale/duplicate messages
// cannot emit another completion. Malformed/regressing output is an error.
func (t *Traversal) Acknowledge(a OutputAcknowledgement) (*TraversalCompletion, error) {
	s := t.schedule
	if a.Generation != s.Generation || a.Sequence != s.Sequence || t.completed {
		return nil, nil
	}
	if a.ConsumedOutputFrame < t.consumed || a.ConsumedOutputFrame > s.OutputEnd {
		return nil, errors.New("invalid output acknowledgement position")
	}
	t.consumed = a.ConsumedOutputFrame
	if t.consumed != s.OutputEnd {
		return nil, nil
	}
	t.completed = true
	return &TraversalCompletion{Type: "traversal_completed", Generation: s.Generation, Sequence: s.Sequence, StartFrame: s.StartFrame, EndFrame: s.EndFrame, ConsumedOutputFrame: t.consumed}, nil
}
