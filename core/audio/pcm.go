package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"github.com/NLstn/riffine/core/resource"
)

// PCMBlockFrames bounds decoder work and temporary storage independently of song
// length. Hosts serialize calls on a control/analysis worker, never a callback.
const PCMBlockFrames = 8192

// OpenPCM borrows the original resource. Closing the stream releases its cursor,
// not the reader; the resource owner must close the reader after the stream.
func (WAVDecoder) OpenPCM(ctx context.Context, source resource.Reader) (PCMStream, error) {
	info, err := Inspect(ctx, source)
	if err != nil {
		return nil, err
	}
	return &wavPCM{source: source, info: info, raw: make([]byte, PCMBlockFrames*info.Channels*info.Bits/8)}, nil
}

type wavPCM struct {
	mu     sync.Mutex
	source resource.Reader
	info   Info
	frame  int64
	raw    []byte
	closed bool
}

func (s *wavPCM) ReadFrames(ctx context.Context, dst []float32) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if s.closed {
		return 0, errors.New("PCM stream is closed")
	}
	channels := s.info.Channels
	if len(dst) == 0 || len(dst)%channels != 0 || len(dst)/channels > PCMBlockFrames {
		return 0, errors.New("PCM buffer must contain 1–8192 complete frames")
	}
	if s.frame == s.info.Frames {
		return 0, io.EOF
	}
	frames := min(int64(len(dst)/channels), s.info.Frames-s.frame)
	bytesPerSample := s.info.Bits / 8
	align := channels * bytesPerSample
	raw := s.raw[:int(frames)*align]
	n, err := (contextReader{ctx, s.source}).ReadAt(raw, s.info.DataOffset+s.frame*int64(align))
	if n != len(raw) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return 0, err
	}
	if err != nil && err != io.EOF {
		return 0, err
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	for i := 0; i < int(frames)*channels; i++ {
		b := raw[i*bytesPerSample:]
		switch s.info.Bits {
		case 16:
			dst[i] = float32(int16(binary.LittleEndian.Uint16(b))) / 32768
		case 24:
			v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			dst[i] = float32((v<<8)>>8) / 8388608
		case 32:
			dst[i] = float32(float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648)
		}
	}
	s.frame += frames
	return int(frames), nil
}
func (s *wavPCM) SeekFrame(ctx context.Context, frame int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return errors.New("PCM stream is closed")
	}
	if frame < 0 || frame > s.info.Frames {
		return errors.New("PCM seek exceeds source frame range")
	}
	s.frame = frame
	return nil
}
func (s *wavPCM) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.raw = nil
	return nil
}

var _ PCMDecoder = WAVDecoder{}
