// Package timing compiles source-time beat grids on a control worker. Hosts
// render the resulting device-frame events; no client or callback computes BPM.
package timing

import (
	"context"
	"errors"
	"math"
	"slices"
	"sort"
)

const Version = 1
const MaxMarkers = 4096
const MaxBatchClicks = 256
const MaxCountInBeats = 32

// Beat ordinals are zero-based quarter-note pulses. Accent is determined by the
// ordinal modulo BeatsPerBar; pitch changes never affect this source-time grid.
type Beat struct {
	SourceFrame int64 `json:"sourceFrame"`
	Ordinal     int64 `json:"ordinal"`
}
type Definition struct {
	Version     int     `json:"version"`
	SourceBPM   float64 `json:"sourceBPM,omitempty"`   // original-recording BPM, 20..400
	OriginFrame int64   `json:"originFrame,omitempty"` // ordinal zero in manual mode
	BeatsPerBar int     `json:"beatsPerBar"`           // 1..16 quarter-note pulses per bar
	Markers     []Beat  `json:"markers,omitempty"`     // explicit, variable-time beat map
}
type Grid struct {
	definition   Definition
	sourceRate   int
	sourceFrames int64
}
type Selection struct {
	StartFrame int64   `json:"startFrame"`
	EndFrame   int64   `json:"endFrame"` // exclusive
	OutputRate int     `json:"outputRate"`
	Speed      float64 `json:"speed"`
}
type Click struct {
	OutputFrame int64 `json:"outputFrame"` // relative to this selection, device frames
	Ordinal     int64 `json:"ordinal"`
	Accent      bool  `json:"accent"`
}
type Batch struct {
	Version         int     `json:"version"`
	Clicks          []Click `json:"clicks"`
	NextOutputFrame int64   `json:"nextOutputFrame"` // exclusive end of queried window
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func Compile(d Definition, sourceRate int, sourceFrames int64) (*Grid, error) {
	if d.Version != Version || sourceRate < 8000 || sourceRate > 192000 || sourceFrames < 1 || sourceFrames > 1<<48 || d.BeatsPerBar < 1 || d.BeatsPerBar > 16 || d.OriginFrame < 0 || d.OriginFrame >= sourceFrames || len(d.Markers) > MaxMarkers {
		return nil, errors.New("invalid beat grid version, source dimensions, meter or marker count")
	}
	if len(d.Markers) == 0 {
		if !finite(d.SourceBPM) || d.SourceBPM < 20 || d.SourceBPM > 400 {
			return nil, errors.New("original-recording BPM must be 20–400")
		}
	} else {
		if d.SourceBPM != 0 || d.OriginFrame != 0 {
			return nil, errors.New("beat map must not also specify manual BPM/origin")
		}
		for i, beat := range d.Markers {
			if beat.SourceFrame < 0 || beat.SourceFrame > sourceFrames || beat.Ordinal < 0 || beat.Ordinal > 1<<48 || i > 0 && (beat.SourceFrame <= d.Markers[i-1].SourceFrame || beat.Ordinal <= d.Markers[i-1].Ordinal) {
				return nil, errors.New("beat-map source frames and ordinals must increase within the source")
			}
		}
	}
	d.Markers = slices.Clone(d.Markers)
	return &Grid{d, sourceRate, sourceFrames}, nil
}
func (g *Grid) Definition() Definition {
	d := g.definition
	d.Markers = slices.Clone(d.Markers)
	return d
}
func (g *Grid) OutputFrames(s Selection) (int64, error) {
	if s.StartFrame < 0 || s.EndFrame <= s.StartFrame || s.EndFrame > g.sourceFrames || s.OutputRate < 8000 || s.OutputRate > 192000 || !finite(s.Speed) || s.Speed < .25 || s.Speed > 2 {
		return 0, errors.New("invalid beat selection/rate/speed")
	}
	return max(1, int64(math.Round(float64(s.EndFrame-s.StartFrame)*float64(s.OutputRate)/float64(g.sourceRate)/s.Speed))), nil
}
func (g *Grid) outputFrame(s Selection, source float64) int64 {
	return int64(math.Round((source - float64(s.StartFrame)) * float64(s.OutputRate) / float64(g.sourceRate) / s.Speed))
}

// Clicks returns a bounded output-frame window, including the tails' STARTS only.
// Use overlapping synthesis windows for click tails. No beat is extrapolated
// outside an explicit map. Independent rounding avoids cumulative tempo drift.
// Oversized windows fail atomically; callers can shorten windows and retry.
func (g *Grid) Clicks(ctx context.Context, s Selection, outputStart, outputEnd int64) (Batch, error) {
	result := Batch{Version: Version, Clicks: []Click{}, NextOutputFrame: outputEnd}
	total, err := g.OutputFrames(s)
	if err != nil {
		return Batch{}, err
	}
	if outputStart < 0 || outputEnd <= outputStart || outputEnd > total {
		return Batch{}, errors.New("beat query must be within the output selection")
	}
	if err = ctx.Err(); err != nil {
		return Batch{}, err
	}
	add := func(source float64, ordinal int64) error {
		if source < float64(s.StartFrame) || source >= float64(s.EndFrame) {
			return nil
		}
		frame := g.outputFrame(s, source)
		if frame < outputStart || frame >= outputEnd {
			return nil
		}
		if len(result.Clicks) == MaxBatchClicks {
			return errors.New("beat window exceeds 256 clicks; query a shorter output window")
		}
		result.Clicks = append(result.Clicks, Click{frame, ordinal, ordinal%int64(g.definition.BeatsPerBar) == 0})
		return nil
	}
	if len(g.definition.Markers) > 0 {
		// One output-frame of slack covers independent nearest-frame rounding.
		sourceStart := float64(s.StartFrame) + float64(max(0, outputStart-1))*float64(g.sourceRate)*s.Speed/float64(s.OutputRate)
		first := sort.Search(len(g.definition.Markers), func(i int) bool { return float64(g.definition.Markers[i].SourceFrame) >= sourceStart })
		for _, beat := range g.definition.Markers[first:] {
			if err = ctx.Err(); err != nil {
				return Batch{}, err
			}
			if g.outputFrame(s, float64(beat.SourceFrame)) >= outputEnd {
				break
			}
			if err = add(float64(beat.SourceFrame), beat.Ordinal); err != nil {
				return Batch{}, err
			}
		}
	} else {
		period := 60 * float64(g.sourceRate) / g.definition.SourceBPM
		sourceStart := float64(s.StartFrame) + float64(max(0, outputStart-1))*float64(g.sourceRate)*s.Speed/float64(s.OutputRate)
		first := max(int64(0), int64(math.Floor((sourceStart-float64(g.definition.OriginFrame))/period)))
		// At most one beat just before the window plus its bounded output clicks.
		for ordinal := first; ; ordinal++ {
			if err = ctx.Err(); err != nil {
				return Batch{}, err
			}
			source := float64(g.definition.OriginFrame) + float64(ordinal)*period
			if source >= float64(s.EndFrame) || g.outputFrame(s, source) >= outputEnd {
				break
			}
			if err = add(source, ordinal); err != nil {
				return Batch{}, err
			}
		}
	}
	return result, nil
}

// CountIn compiles a finite preparation phase at the original recording's local
// tempo, scaled by selection speed. A variable map needs two consecutive beat
// ordinals to establish that tempo; it never guesses from speed alone.
func (g *Grid) CountIn(ctx context.Context, s Selection, beats int) (Batch, int64, error) {
	if _, err := g.OutputFrames(s); err != nil {
		return Batch{}, 0, err
	}
	if beats < 1 || beats > MaxCountInBeats {
		return Batch{}, 0, errors.New("count-in requires 1–32 beats")
	}
	sourcePeriod := 0.0
	if len(g.definition.Markers) == 0 {
		sourcePeriod = 60 * float64(g.sourceRate) / g.definition.SourceBPM
	}
	if len(g.definition.Markers) > 0 {
		if len(g.definition.Markers) < 2 {
			return Batch{}, 0, errors.New("count-in needs two consecutive beat markers or manual BPM")
		}
		i := sort.Search(len(g.definition.Markers), func(i int) bool { return g.definition.Markers[i].SourceFrame > s.StartFrame })
		i = max(0, i-1)
		if i >= len(g.definition.Markers)-1 {
			i = len(g.definition.Markers) - 2
		}
		a, b := g.definition.Markers[i], g.definition.Markers[i+1]
		if b.Ordinal != a.Ordinal+1 {
			return Batch{}, 0, errors.New("count-in local tempo needs consecutive beat ordinals")
		}
		sourcePeriod = float64(b.SourceFrame - a.SourceFrame)
	}
	devicePeriod := sourcePeriod * float64(s.OutputRate) / float64(g.sourceRate) / s.Speed
	if devicePeriod < 1 || devicePeriod*float64(beats) > float64(s.OutputRate)*120 {
		return Batch{}, 0, errors.New("count-in local tempo exceeds timing bounds")
	}
	frames := int64(math.Round(devicePeriod * float64(beats)))
	result := Batch{Version: Version, Clicks: make([]Click, 0, beats), NextOutputFrame: frames}
	for i := 0; i < beats; i++ {
		if err := ctx.Err(); err != nil {
			return Batch{}, 0, err
		}
		result.Clicks = append(result.Clicks, Click{int64(math.Round(float64(i) * devicePeriod)), int64(i), i%g.definition.BeatsPerBar == 0})
	}
	return result, frames, nil
}
