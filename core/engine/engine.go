// Package engine defines host-neutral offline processing and playback ports.
package engine

import (
	"context"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

// Source and Destination are opaque host resource handles, not core paths.
// The native adapter interprets them as local paths. Ranges are source frames
// with inclusive start and exclusive end. Rendering does not emit loop events.
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

// PreviewRequest describes one original-source range, with fixed parameters.
// It is independent of transport, loop repetition and output acknowledgements.
type PreviewRequest struct {
	Source               string
	StartFrame, EndFrame int64
	Speed, Pitch         float64
}

// PreviewReport is diagnostic evidence, not a playback-completion event.
// CopiedFrames and SampleRate refer to the callback's source-rate PCM, before
// native device conversion/buffering; they are not hardware presentation frames.
// StartupMilliseconds excludes core source identity verification.
type PreviewReport struct {
	Version             int     `json:"version"`
	Channels            int     `json:"channels"`
	SampleRate          int     `json:"sampleRate"`
	OutputFrames        int64   `json:"outputFrames"`
	CopiedFrames        int64   `json:"copiedFrames"`
	UnderrunFrames      int64   `json:"underrunFrames"`
	DecodedBeforeStart  int64   `json:"decodedBeforeStart"`
	StartupMilliseconds float64 `json:"startupMilliseconds"`
	Backend             string  `json:"backend"`
	Mode                string  `json:"mode"`
}

type Previewer interface {
	Preview(context.Context, PreviewRequest) (PreviewReport, error)
}

// SourcePreviewer borrows a stable, identity-verified reader and inspected
// metadata for one preview. It closes its own decoder cursor before returning,
// never the caller's reader. Calls run on a control worker, never an audio callback.
type SourcePreviewer interface {
	PreviewSource(context.Context, resource.Reader, audio.Info, PreviewRequest) (PreviewReport, error)
}
