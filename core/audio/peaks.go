package audio

import (
	"context"
	"errors"
	"io"
	"math"
	"math/bits"
)

// PeaksFromPCM is the shared flat-waveform algorithm for every decoder port.
// It borrows a stream positioned at frame zero; the caller owns its close.
func PeaksFromPCM(ctx context.Context, info Info, stream PCMStream, bins int) (Waveform, error) {
	if bins < 1 || bins > 100000 {
		return Waveform{}, errors.New("bins must be 1–100000")
	}
	if err := ValidateInfo(info); err != nil {
		return Waveform{}, err
	}
	bins = int(min(int64(bins), info.Frames))
	w := Waveform{SampleRate: info.SampleRate, Frames: info.Frames, Channels: make([][]Peak, info.Channels)}
	for c := range w.Channels {
		w.Channels[c] = make([]Peak, bins)
		for j := range w.Channels[c] {
			w.Channels[c][j] = Peak{Min: float32(math.Inf(1)), Max: float32(math.Inf(-1))}
		}
	}
	samples := make([]float32, PCMBlockFrames*info.Channels)
	for frame := int64(0); frame < info.Frames; {
		if err := ctx.Err(); err != nil {
			return Waveform{}, err
		}
		n, err := stream.ReadFrames(ctx, samples)
		if err != nil && err != io.EOF {
			return Waveform{}, err
		}
		if n < 1 || n > PCMBlockFrames || int64(n) > info.Frames-frame {
			return Waveform{}, errors.New("invalid or truncated waveform PCM")
		}
		for j := 0; j < n; j++ {
			hi, lo := bits.Mul64(uint64(frame+int64(j)), uint64(bins))
			q, _ := bits.Div64(hi, lo, uint64(info.Frames))
			bin := int(q)
			for c := 0; c < info.Channels; c++ {
				v := samples[j*info.Channels+c]
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					return Waveform{}, errors.New("non-finite waveform PCM")
				}
				p := &w.Channels[c][bin]
				p.Min = min(p.Min, v)
				p.Max = max(p.Max, v)
			}
		}
		frame += int64(n)
		if err == io.EOF && frame != info.Frames {
			return Waveform{}, errors.New("waveform ended before advertised source length")
		}
	}
	if err := ctx.Err(); err != nil {
		return Waveform{}, err
	}
	return w, nil
}
