// Package waveform builds bounded source-coordinate peak pyramids on workers.
package waveform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

const AlgorithmVersion = 1
const MaxBaseBins = 65536
const MaxWindowBins = 4096
const MaxCacheBytes = 4 << 20

// Cache keys are opaque algorithm/content identities. Put atomically publishes
// complete bytes, replacing a corrupt cache if necessary. It never writes audio.
type Cache interface {
	Get(context.Context, string, int) ([]byte, error)
	Put(context.Context, string, []byte) error
}
type Progress struct {
	DecodedFrames int64 `json:"decodedFrames"`
	TotalFrames   int64 `json:"totalFrames"`
}
type Result struct {
	Pyramid     *Pyramid
	CacheStatus string
	CacheError  error
}

// Pyramid is immutable after construction. Query allocates only its bounded
// result and never reads source audio; a changed resource must be opened again.
type Pyramid struct {
	info   audio.Info
	base   int64
	levels [][][]audio.Peak
}
type Cell struct {
	StartFrame int64        `json:"startFrame"`
	EndFrame   int64        `json:"endFrame"`
	Channels   []audio.Peak `json:"channels"`
}
type Window struct {
	Version        int    `json:"version"`
	SHA256         string `json:"sha256"`
	SampleRate     int    `json:"sampleRate"`
	SourceFrames   int64  `json:"sourceFrames"`
	RequestedStart int64  `json:"requestedStart"`
	RequestedEnd   int64  `json:"requestedEnd"`
	FramesPerCell  int64  `json:"framesPerCell"`
	Cells          []Cell `json:"cells"`
}

func baseFrames(frames int64) int64 {
	base := int64(256)
	for (frames+base-1)/base > MaxBaseBins {
		base *= 2
	}
	return base
}
func key(info audio.Info) string {
	return fmt.Sprintf("wave-v%d-%s-%d-%d-%d", AlgorithmVersion, info.SHA256, info.SampleRate, info.Channels, info.Frames)
}

// Open fingerprints the stable resource, then loads or builds a bounded pyramid.
// Generation is cancellable between decoder reads and progress notifications.
// Cache failures are returned as diagnostics while the computed waveform remains
// usable. Interrupted generation is never published.
func Open(ctx context.Context, source resource.Reader, decoder interface {
	audio.Decoder
	audio.PCMDecoder
}, cache Cache, progress func(Progress)) (Result, error) {
	var result Result
	if source == nil || decoder == nil {
		return result, errors.New("waveform resource and decoder ports are required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	info, err := decoder.Inspect(ctx, source)
	if err != nil {
		return result, err
	}
	return OpenWithInfo(ctx, source, decoder, info, cache, progress)
}

// OpenWithInfo accepts metadata already fingerprinted and verified by the core.
// The caller guarantees a stable reader matching that exact original identity.
func OpenWithInfo(ctx context.Context, source resource.Reader, decoder audio.PCMDecoder, info audio.Info, cache Cache, progress func(Progress)) (Result, error) {
	var result Result
	if source == nil || decoder == nil {
		return result, errors.New("waveform resource and decoder ports are required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var err error
	if err = validateInfo(info); err != nil {
		return result, err
	}
	if cache != nil {
		data, e := cache.Get(ctx, key(info), MaxCacheBytes)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if e == nil {
			if p, e := decode(data, info); e == nil {
				if err := ctx.Err(); err != nil {
					return result, err
				}
				result.Pyramid = p
				result.CacheStatus = "hit"
				return result, nil
			} else {
				result.CacheError = e
			}
		} else {
			result.CacheError = e
		}
	}
	var stream audio.PCMStream
	if verified, ok := decoder.(audio.VerifiedPCMDecoder); ok {
		stream, err = verified.OpenVerifiedPCM(ctx, source, info)
	} else {
		stream, err = decoder.OpenPCM(ctx, source)
	}
	if err != nil {
		return result, err
	}
	closed := false
	defer func() {
		if !closed {
			stream.Close()
		}
	}()
	p := &Pyramid{info: info, base: baseFrames(info.Frames)}
	count := int((info.Frames + p.base - 1) / p.base)
	level := make([][]audio.Peak, info.Channels)
	for c := range level {
		level[c] = make([]audio.Peak, count)
		for j := range level[c] {
			level[c][j] = audio.Peak{Min: float32(math.Inf(1)), Max: float32(math.Inf(-1))}
		}
	}
	samples := make([]float32, audio.PCMBlockFrames*info.Channels)
	var frame int64
	if progress != nil {
		progress(Progress{0, info.Frames})
	}
	for frame < info.Frames {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		n, e := stream.ReadFrames(ctx, samples)
		if n < 1 || n > audio.PCMBlockFrames || int64(n) > info.Frames-frame {
			return result, errors.New("decoder returned invalid or truncated waveform frames")
		}
		if e != nil && e != io.EOF {
			return result, e
		}
		for j := 0; j < n; j++ {
			b := int((frame + int64(j)) / p.base)
			for c := 0; c < info.Channels; c++ {
				v := samples[j*info.Channels+c]
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					return result, errors.New("non-finite waveform PCM")
				}
				peak := &level[c][b]
				peak.Min = min(peak.Min, v)
				peak.Max = max(peak.Max, v)
			}
		}
		frame += int64(n)
		if progress != nil {
			progress(Progress{frame, info.Frames})
		}
		if e == io.EOF && frame != info.Frames {
			return result, errors.New("waveform source ended before advertised length")
		}
	}
	p.levels = append(p.levels, level)
	for count > 1 {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		parents := make([][]audio.Peak, info.Channels)
		for c := range parents {
			parents[c] = make([]audio.Peak, (count+1)/2)
			for j := range parents[c] {
				a := level[c][2*j]
				if 2*j+1 < count {
					b := level[c][2*j+1]
					a.Min = min(a.Min, b.Min)
					a.Max = max(a.Max, b.Max)
				}
				parents[c][j] = a
			}
		}
		p.levels = append(p.levels, parents)
		level = parents
		count = (count + 1) / 2
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	err = stream.Close()
	closed = true
	if err != nil {
		return result, err
	}
	result.Pyramid = p
	result.CacheStatus = "disabled"
	if cache != nil {
		result.CacheStatus = "rebuilt"
		if e := cache.Put(ctx, key(info), encode(p)); e != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			result.CacheError = e
		} else {
			result.CacheError = nil
		}
	}
	return result, nil
}

// Query returns complete source-aligned cells intersecting [start,end). Cells
// may extend outside the requested viewport; their actual coordinates are never
// relabelled as exact selection extrema. The client clips drawing to its view.
// maxBins is a strict result bound, including partial cells at either edge.
func (p *Pyramid) Query(ctx context.Context, start, end int64, maxBins int) (Window, error) {
	var w Window
	if err := ctx.Err(); err != nil {
		return w, err
	}
	if p == nil || start < 0 || end <= start || end > p.info.Frames || maxBins < 1 || maxBins > MaxWindowBins {
		return w, errors.New("invalid waveform window or bin limit")
	}
	level := 0
	span := p.base
	for (end-1)/span-start/span+1 > int64(maxBins) {
		level++
		span *= 2
	}
	first, last := start/span, (end-1)/span
	w = Window{Version: AlgorithmVersion, SHA256: p.info.SHA256, SampleRate: p.info.SampleRate, SourceFrames: p.info.Frames, RequestedStart: start, RequestedEnd: end, FramesPerCell: span, Cells: make([]Cell, 0, int(last-first+1))}
	for j := first; j <= last; j++ {
		if err := ctx.Err(); err != nil {
			return Window{}, err
		}
		cell := Cell{StartFrame: j * span, EndFrame: min((j+1)*span, p.info.Frames), Channels: make([]audio.Peak, p.info.Channels)}
		for c := range cell.Channels {
			cell.Channels[c] = p.levels[level][c][j]
		}
		w.Cells = append(w.Cells, cell)
	}
	return w, nil
}
