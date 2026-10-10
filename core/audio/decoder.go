package audio

import (
	"context"
	"github.com/NLstn/riffine/core/resource"
)

// Decoder operates on borrowed resources: the caller owns and closes readers.
// Implementations report only formats tested by that host. Inspect computes
// SHA-256 of the complete original bytes, including headers and tags.
type Decoder interface {
	Formats() []string
	Inspect(context.Context, resource.Reader) (Info, error)
	Peaks(context.Context, resource.Reader, int) (Waveform, error)
}

type WAVDecoder struct{}

func (WAVDecoder) Formats() []string { return []string{"wav-pcm"} }
func (WAVDecoder) Inspect(ctx context.Context, r resource.Reader) (Info, error) {
	return Inspect(ctx, r)
}
func (WAVDecoder) Peaks(ctx context.Context, r resource.Reader, bins int) (Waveform, error) {
	return Peaks(ctx, r, bins)
}

// PCMDecoder is the future streaming decoder port. Frames are interleaved,
// normalized float32 in original source time; ReadFrames returns per-channel
// frame counts, and SeekFrame is exact after decoder offset compensation.
// WAVDecoder currently implements inspection/peaks only.
type PCMDecoder interface {
	OpenPCM(context.Context, resource.Reader) (PCMStream, error)
}
type PCMStream interface {
	ReadFrames(context.Context, []float32) (int, error)
	SeekFrame(context.Context, int64) error
	Close() error
}
