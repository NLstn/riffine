package practice

import (
	"context"
	"errors"
	"sync"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
	"github.com/NLstn/riffine/core/waveform"
)

// WaveformDetail owns its source resource and analysis cursor independently of
// project edits. Close it before releasing host resource access or relinking.
type WaveformDetail struct {
	detail   *waveform.Detail
	source   resource.Reader
	once     sync.Once
	closeErr error
}

func (w *WaveformDetail) Query(ctx context.Context, start, end int64, maxBins int) (waveform.Window, error) {
	return w.detail.Query(ctx, start, end, maxBins)
}
func (w *WaveformDetail) Close() error {
	w.once.Do(func() { w.closeErr = errors.Join(w.detail.Close(), w.source.Close()) })
	return w.closeErr
}

// OpenWaveformDetail verifies identity once. Repeated detail queries retain the
// same reader and decoder, using source frames regardless of tempo or pitch.
func (s *Session) OpenWaveformDetail(ctx context.Context) (*WaveformDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decoder, ok := s.service.Decoder.(audio.PCMDecoder)
	if !ok {
		return nil, failure(UnsupportedCapability, errors.New("waveform detail decoding is unavailable"))
	}
	source, err := s.openVerified(ctx, s.source)
	if err != nil {
		return nil, err
	}
	stream, err := decoder.OpenPCM(ctx, source)
	if err != nil {
		source.Close()
		return nil, failure(HostFailure, err)
	}
	detail, err := waveform.NewDetail(s.p.Track.Audio, stream)
	if err != nil {
		stream.Close()
		source.Close()
		return nil, failure(HostFailure, err)
	}
	return &WaveformDetail{detail: detail, source: source}, nil
}
