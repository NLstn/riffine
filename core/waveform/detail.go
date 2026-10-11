package waveform

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"

	"github.com/NLstn/riffine/core/audio"
)

// MaxDetailFrames bounds the decoded viewport work per request, independently
// of song length. Wider views use Pyramid.Query. Compressed codec seek work may
// include a source prefix; the decoder's capability documentation defines that.
const MaxDetailFrames int64 = 1 << 20

// Detail owns a decoder cursor on a stable, already verified original resource.
// Keep this handle across zoom requests; queries do not reopen or fingerprint
// the resource. The host closes the handle before releasing the borrowed source.
// Calls are serialized on an analysis worker, never the realtime callback.
type Detail struct {
	mu             sync.Mutex
	info           audio.Info
	stream         audio.PCMStream
	samples        []float32
	closed, failed bool
}

// NewDetail takes ownership of stream on success. On failure the caller retains
// ownership. Its resource must match info and remain stable until Close.
func NewDetail(info audio.Info, stream audio.PCMStream) (*Detail, error) {
	if stream == nil {
		return nil, errors.New("waveform detail requires a PCM cursor")
	}
	if err := validateInfo(info); err != nil {
		return nil, err
	}
	return &Detail{info: info, stream: stream, samples: make([]float32, audio.PCMBlockFrames*info.Channels)}, nil
}

// Query returns extrema only within [start,end), unlike overview pyramid cells
// which may extend beyond the viewport. Up to maxBins owned cells cover the exact
// range, and one-frame cells contain that sample's value in both Min and Max.
// A cancelled or failed decoder operation requires closing/reopening the handle;
// rejected requests and cancellation before decoding leave it usable.
func (d *Detail) Query(ctx context.Context, start, end int64, maxBins int) (Window, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Window{}, err
	}
	if d.closed || d.failed {
		return Window{}, errors.New("waveform detail is closed or failed; reopen it")
	}
	if start < 0 || end <= start || end > d.info.Frames || end-start > MaxDetailFrames || maxBins < 1 || maxBins > MaxWindowBins {
		return Window{}, errors.New("invalid detail range or bin limit; wider views require the peak pyramid")
	}
	span := (end - start + int64(maxBins) - 1) / int64(maxBins)
	count := int((end - start + span - 1) / span)
	w := Window{Version: AlgorithmVersion, SHA256: d.info.SHA256, SampleRate: d.info.SampleRate, SourceFrames: d.info.Frames, RequestedStart: start, RequestedEnd: end, FramesPerCell: span, Cells: make([]Cell, count)}
	for j := range w.Cells {
		cell := Cell{StartFrame: start + int64(j)*span, EndFrame: min(start+int64(j+1)*span, end), Channels: make([]audio.Peak, d.info.Channels)}
		for c := range cell.Channels {
			cell.Channels[c] = audio.Peak{Min: float32(math.Inf(1)), Max: float32(math.Inf(-1))}
		}
		w.Cells[j] = cell
	}
	if err := d.stream.SeekFrame(ctx, start); err != nil {
		d.failed = true
		return Window{}, err
	}
	for frame := start; frame < end; {
		remaining := int(min(int64(audio.PCMBlockFrames), end-frame))
		n, err := d.stream.ReadFrames(ctx, d.samples[:remaining*d.info.Channels])
		if err != nil && err != io.EOF {
			d.failed = true
			return Window{}, err
		}
		if n < 1 || n > remaining {
			d.failed = true
			return Window{}, errors.New("invalid or truncated waveform detail PCM")
		}
		for j := 0; j < n; j++ {
			cell := &w.Cells[int((frame+int64(j)-start)/span)]
			for c := range cell.Channels {
				value := d.samples[j*d.info.Channels+c]
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					d.failed = true
					return Window{}, errors.New("non-finite waveform detail PCM")
				}
				cell.Channels[c].Min = min(cell.Channels[c].Min, value)
				cell.Channels[c].Max = max(cell.Channels[c].Max, value)
			}
		}
		frame += int64(n)
		if err == io.EOF && frame != end {
			d.failed = true
			return Window{}, io.ErrUnexpectedEOF
		}
		if err := ctx.Err(); err != nil {
			d.failed = true
			return Window{}, err
		}
	}
	return w, nil
}

func (d *Detail) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.samples = nil
	return d.stream.Close()
}
