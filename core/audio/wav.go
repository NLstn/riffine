// Package audio provides bounded PCM WAV import and waveform extraction.
package audio

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/NLstn/riffine/core/resource"
)

// Info describes an interleaved integer PCM WAV. Frames are per channel.
type Info struct {
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
	Bits       int    `json:"bits"`
	Frames     int64  `json:"frames"`
	SHA256     string `json:"sha256"`
	DataOffset int64  `json:"-"`
	DataBytes  int64  `json:"-"`
}

func (i Info) Duration() float64 { return float64(i.Frames) / float64(i.SampleRate) }

// Inspect accepts RIFF/WAVE PCM 16/24/32-bit, mono/stereo at 8–192 kHz.
// It streams the fingerprint and checks chunk boundaries without loading audio.
func Inspect(ctx context.Context, source resource.Reader) (Info, error) {
	f := io.NewSectionReader(contextReader{ctx, source}, 0, source.Size())
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	var err error
	var header [12]byte
	if _, err = io.ReadFull(f, header[:]); err != nil {
		return Info{}, err
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return Info{}, errors.New("expected RIFF/WAVE file")
	}
	end := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if end != source.Size() || end < 12 {
		return Info{}, errors.New("invalid RIFF size")
	}
	var info Info
	var formatFound, dataFound bool
	for pos := int64(12); pos < end; {
		if end-pos < 8 {
			return Info{}, errors.New("truncated WAV chunk")
		}
		var chunk [8]byte
		if _, err = f.ReadAt(chunk[:], pos); err != nil {
			return Info{}, err
		}
		n := int64(binary.LittleEndian.Uint32(chunk[4:]))
		start := pos + 8
		if n > end-start {
			return Info{}, errors.New("WAV chunk exceeds file")
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if formatFound || n < 16 {
				return Info{}, errors.New("invalid or duplicate WAV format")
			}
			var b [16]byte
			if _, err = f.ReadAt(b[:], start); err != nil {
				return Info{}, err
			}
			if binary.LittleEndian.Uint16(b[:2]) != 1 {
				return Info{}, errors.New("only integer PCM WAV is supported (convert float, compressed or extensible WAV first)")
			}
			info.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
			info.SampleRate = int(binary.LittleEndian.Uint32(b[4:8]))
			info.Bits = int(binary.LittleEndian.Uint16(b[14:16]))
			if info.Channels < 1 || info.Channels > 2 || info.SampleRate < 8000 || info.SampleRate > 192000 || (info.Bits != 16 && info.Bits != 24 && info.Bits != 32) {
				return Info{}, errors.New("supported WAV: mono/stereo, 8–192 kHz, PCM 16/24/32-bit")
			}
			align := info.Channels * info.Bits / 8
			if int(binary.LittleEndian.Uint16(b[12:14])) != align || int(binary.LittleEndian.Uint32(b[8:12])) != info.SampleRate*align {
				return Info{}, errors.New("invalid WAV alignment or byte rate")
			}
			formatFound = true
		case "data":
			if dataFound {
				return Info{}, errors.New("multiple WAV data chunks unsupported")
			}
			info.DataOffset = start
			info.DataBytes = n
			dataFound = true
		}
		pos = start + n + (n % 2)
		if pos > end {
			return Info{}, errors.New("missing WAV chunk padding")
		}
	}
	if !formatFound || !dataFound {
		return Info{}, errors.New("missing WAV format or data")
	}
	align := int64(info.Channels * info.Bits / 8)
	if info.DataBytes == 0 || info.DataBytes%align != 0 {
		return Info{}, errors.New("empty or partial WAV frames")
	}
	info.Frames = info.DataBytes / align
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Info{}, err
	}
	if n != source.Size() {
		return Info{}, errors.New("audio changed or truncated while fingerprinting")
	}
	info.SHA256 = hex.EncodeToString(h.Sum(nil))
	return info, nil
}

type Peak struct {
	Min float32 `json:"min"`
	Max float32 `json:"max"`
}
type Waveform struct {
	SampleRate int      `json:"sampleRate"`
	Frames     int64    `json:"frames"`
	Channels   [][]Peak `json:"channels"`
}

// Peaks preserves per-channel min/max amplitudes with bounded memory.
func Peaks(ctx context.Context, source resource.Reader, bins int) (Waveform, error) {
	if bins < 1 || bins > 100000 {
		return Waveform{}, errors.New("bins must be 1–100000")
	}
	info, err := Inspect(ctx, source)
	if err != nil {
		return Waveform{}, err
	}
	if int64(bins) > info.Frames {
		bins = int(info.Frames)
	}
	w := Waveform{SampleRate: info.SampleRate, Frames: info.Frames, Channels: make([][]Peak, info.Channels)}
	for c := range w.Channels {
		w.Channels[c] = make([]Peak, bins)
		for b := range w.Channels[c] {
			w.Channels[c][b] = Peak{Min: 1, Max: -1}
		}
	}
	f := io.NewSectionReader(contextReader{ctx, source}, info.DataOffset, info.DataBytes)
	bytesPerSample := info.Bits / 8
	align := bytesPerSample * info.Channels
	buf := make([]byte, 4096*align)
	for frame := int64(0); frame < info.Frames; {
		n := min(int64(4096), info.Frames-frame)
		if _, err = io.ReadFull(f, buf[:int(n)*align]); err != nil {
			return Waveform{}, err
		}
		for j := int64(0); j < n; j++ {
			bin := int((frame + j) * int64(bins) / info.Frames)
			for c := 0; c < info.Channels; c++ {
				b := buf[int(j)*align+c*bytesPerSample:]
				var v float32
				switch info.Bits {
				case 16:
					v = float32(int16(binary.LittleEndian.Uint16(b))) / 32768
				case 24:
					x := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
					x = (x << 8) >> 8
					v = float32(x) / 8388608
				case 32:
					v = float32(float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648)
				}
				p := &w.Channels[c][bin]
				p.Min = min(p.Min, v)
				p.Max = max(p.Max, v)
			}
		}
		frame += n
	}
	return w, nil
}

// WriteTone writes a deterministic PCM16 sine fixture without copyrighted audio.
func WriteTone(f io.Writer, seconds, hz float64, sampleRate, channels int) error {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 60 || math.IsNaN(hz) || math.IsInf(hz, 0) || hz <= 0 || hz >= float64(sampleRate)/2 || sampleRate < 8000 || sampleRate > 192000 || (channels != 1 && channels != 2) {
		return errors.New("invalid tone parameters")
	}
	frames := int(seconds * float64(sampleRate))
	size := frames * channels * 2
	if frames == 0 {
		return fmt.Errorf("tone shorter than one frame")
	}
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+size))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], uint16(channels))
	binary.LittleEndian.PutUint32(h[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(h[28:], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(h[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(size))
	if n, err := f.Write(h); err != nil {
		return err
	} else if n != len(h) {
		return io.ErrShortWrite
	}
	b := make([]byte, size)
	for i := 0; i < frames; i++ {
		v := int16(0.25 * 32767 * math.Sin(2*math.Pi*hz*float64(i)/float64(sampleRate)))
		for c := 0; c < channels; c++ {
			binary.LittleEndian.PutUint16(b[(i*channels+c)*2:], uint16(v))
		}
	}
	if n, err := f.Write(b); err != nil {
		return err
	} else if n != len(b) {
		return io.ErrShortWrite
	}
	return nil
}

// Reads are bounded and observe cancellation between host reads. A host must
// make an individual blocking read interruptible when its platform requires it.
type contextReader struct {
	ctx    context.Context
	source io.ReaderAt
}

func (r contextReader) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.ReadAt(p, off)
}
