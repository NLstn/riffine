package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
)

func TestPCMIntegerFramesAndSeek(t *testing.T) {
	for _, bits := range []int{16, 24, 32} {
		t.Run(fmt.Sprint(bits), func(t *testing.T) {
			width := bits / 8
			// Stereo endpoints and a silent frame exercise sign extension and channels.
			values := []int64{-1 << (bits - 1), (1 << (bits - 1)) - 1, 0, 0, (1 << (bits - 2)), -1 << (bits - 2)}
			data := make([]byte, len(values)*width)
			for i, v := range values {
				for j := 0; j < width; j++ {
					data[i*width+j] = byte(v >> (8 * j))
				}
			}
			b := make([]byte, 44+len(data))
			copy(b, "RIFF")
			binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
			copy(b[8:], "WAVEfmt ")
			binary.LittleEndian.PutUint32(b[16:], 16)
			binary.LittleEndian.PutUint16(b[20:], 1)
			binary.LittleEndian.PutUint16(b[22:], 2)
			binary.LittleEndian.PutUint32(b[24:], 48000)
			binary.LittleEndian.PutUint32(b[28:], uint32(48000*2*width))
			binary.LittleEndian.PutUint16(b[32:], uint16(2*width))
			binary.LittleEndian.PutUint16(b[34:], uint16(bits))
			copy(b[36:], "data")
			binary.LittleEndian.PutUint32(b[40:], uint32(len(data)))
			copy(b[44:], data)
			ctx := context.Background()
			s, err := (WAVDecoder{}).OpenPCM(ctx, memoryReader{bytes.NewReader(b)})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			got := make([]float32, 10)
			n, err := s.ReadFrames(ctx, got)
			if err != nil || n != 3 {
				t.Fatalf("read %d %v", n, err)
			}
			if got[0] != -1 || got[1] < .999 || got[2] != 0 || got[3] != 0 || got[4] != .5 || got[5] != -.5 {
				t.Fatal(got)
			}
			if n, err = s.ReadFrames(ctx, got); n != 0 || err != io.EOF {
				t.Fatalf("EOF %d %v", n, err)
			}
			if err = s.SeekFrame(ctx, 1); err != nil {
				t.Fatal(err)
			}
			seek := make([]float32, 4)
			n, err = s.ReadFrames(ctx, seek)
			if err != nil || n != 2 || !reflect.DeepEqual(seek, got[2:6]) {
				t.Fatalf("seek %v %d %v", seek, n, err)
			}
			if err = s.SeekFrame(ctx, 3); err != nil {
				t.Fatal(err)
			}
			if err = s.SeekFrame(ctx, 4); err == nil {
				t.Fatal("seek past EOF accepted")
			}
		})
	}
}

func TestPCMBoundsCancellationAndOwnership(t *testing.T) {
	var b bytes.Buffer
	if err := WriteTone(&b, .1, 440, 48000, 2); err != nil {
		t.Fatal(err)
	}
	r := &observedPCMReader{Reader: bytes.NewReader(b.Bytes())}
	ctx := context.Background()
	s, err := (WAVDecoder{}).OpenPCM(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 1, 2 * (PCMBlockFrames + 1)} {
		if _, err = s.ReadFrames(ctx, make([]float32, size)); err == nil {
			t.Fatal("accepted bad buffer", size)
		}
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.ReadFrames(c, make([]float32, 2)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = s.SeekFrame(c, 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r.maxRead = 0
	samples := make([]float32, PCMBlockFrames*2)
	if n, err := s.ReadFrames(ctx, samples); err != nil || n != 4800 {
		t.Fatalf("read %d %v", n, err)
	}
	if samples[0] != 0 || samples[1] != 0 || r.maxRead > PCMBlockFrames*4 {
		t.Fatal("cancel advanced cursor or read was unbounded")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if r.closed {
		t.Fatal("closed borrowed reader")
	}
	if _, err = s.ReadFrames(ctx, samples); err == nil {
		t.Fatal("read after close")
	}
	if err = s.SeekFrame(ctx, 0); err == nil {
		t.Fatal("seek after close")
	}
	if _, err = r.ReadAt(make([]byte, 12), 0); err != nil {
		t.Fatal("resource no longer usable", err)
	}
}

type observedPCMReader struct {
	*bytes.Reader
	closed  bool
	maxRead int
}

func (r *observedPCMReader) Name() string { return "borrowed.wav" }
func (r *observedPCMReader) Close() error { r.closed = true; return nil }
func (r *observedPCMReader) ReadAt(p []byte, o int64) (int, error) {
	r.maxRead = max(r.maxRead, len(p))
	return r.Reader.ReadAt(p, o)
}
