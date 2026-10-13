package timing

import (
	"errors"
	"math"
)

// CueDefinition is saved configuration, never a resumed runtime or loop counter.
// RestSeconds are device-time seconds, independent of song speed and BPM.
type CueDefinition struct {
	Version        int         `json:"version"`
	CountInBeats   int         `json:"countInBeats"`
	RestSeconds    float64     `json:"restSeconds"`
	ClickLevel     float64     `json:"clickLevel"`
	Metronome      bool        `json:"metronome"`
	Grid           *Definition `json:"grid,omitempty"`
	UseBeatMarkers bool        `json:"useBeatMarkers,omitempty"`
}

func (d *CueDefinition) Clone() *CueDefinition {
	if d == nil {
		return nil
	}
	copy := *d
	if d.Grid != nil {
		grid := *d.Grid
		grid.Markers = append([]Beat(nil), grid.Markers...)
		copy.Grid = &grid
	}
	return &copy
}
func (d *CueDefinition) Compile(sourceRate int, sourceFrames int64) (*Grid, error) {
	if d == nil {
		return nil, nil
	}
	if d.UseBeatMarkers {
		return nil, errors.New("resolve saved beat markers through the project before playback")
	}
	if d.Version != Version || d.CountInBeats < 0 || d.CountInBeats > MaxCountInBeats || !finite(d.RestSeconds) || d.RestSeconds < 0 || d.RestSeconds > 120 || !finite(d.ClickLevel) || d.ClickLevel < 0 || d.ClickLevel > 1 {
		return nil, errors.New("cue options require version 1, 0–32 count-in beats, 0–120 rest seconds and click level 0–1")
	}
	if (d.CountInBeats > 0 || d.Metronome) && d.Grid == nil {
		return nil, errors.New("count-in/metronome needs original-recording BPM or explicit beat markers")
	}
	if d.Grid == nil {
		return nil, nil
	}
	return Compile(*d.Grid, sourceRate, sourceFrames)
}
func (d *CueDefinition) Enabled() bool {
	return d != nil && (d.CountInBeats > 0 || d.RestSeconds > 0 || d.Metronome)
}
func (d *CueDefinition) RestFrames(outputRate int) int64 {
	if d == nil || d.RestSeconds == 0 {
		return 0
	}
	return max(1, int64(math.Round(d.RestSeconds*float64(outputRate))))
}
