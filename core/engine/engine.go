// Package engine defines host-neutral offline processing and playback ports.
package engine

import "context"

type RenderRequest struct {
	Source      string
	Destination string
	StartFrame  int64
	EndFrame    int64
	Speed       float64
	Pitch       float64
}

// Renderer must create a PCM WAV and refuse to replace existing output.
// Speed is source frames per output frame; pitch is independent semitones.
type Renderer interface {
	Render(context.Context, RenderRequest) error
}
type Player interface {
	Play(context.Context, string, bool) error
}
