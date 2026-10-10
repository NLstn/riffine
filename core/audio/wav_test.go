package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
)

func TestToneAndPeaks(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteTone(&buffer, 1, 440, 48000, 2); err != nil {
		t.Fatal(err)
	}
	p := memoryReader{bytes.NewReader(buffer.Bytes())}
	i, err := Inspect(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if i.Frames != 48000 || i.Channels != 2 || i.Duration() != 1 || len(i.SHA256) != 64 {
		t.Fatalf("bad metadata: %+v", i)
	}
	w, err := Peaks(context.Background(), p, 17)
	if err != nil {
		t.Fatal(err)
	}
	for c := range w.Channels {
		if len(w.Channels[c]) != 17 {
			t.Fatal("wrong bins")
		}
		for _, peak := range w.Channels[c] {
			if peak.Min > -.24 || peak.Max < .24 {
				t.Fatalf("missed peak: %+v", peak)
			}
		}
	}
	if _, err = Peaks(context.Background(), p, 0); err == nil {
		t.Fatal("accepted zero bins")
	}
}

func TestMalformedWAV(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteTone(&buffer, .1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	original := buffer.Bytes()
	tests := map[string]func([]byte) []byte{
		"truncated":       func(b []byte) []byte { return b[:20] },
		"bad format":      func(b []byte) []byte { b[20] = 3; return b },
		"zero channels":   func(b []byte) []byte { b[22] = 0; return b },
		"oversized chunk": func(b []byte) []byte { binary.LittleEndian.PutUint32(b[40:], ^uint32(0)); return b },
		"bad alignment":   func(b []byte) []byte { b[32] = 7; return b },
		"bad byte rate":   func(b []byte) []byte { b[28] ^= 1; return b },
		"trailing bytes":  func(b []byte) []byte { return append(b, 0) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			b := mutate(append([]byte(nil), original...))
			p := memoryReader{bytes.NewReader(b)}
			if _, err := Inspect(context.Background(), p); err == nil {
				t.Fatal("accepted malformed WAV")
			}
		})
	}
}

func TestIntegerDepthsAndOddChunk(t *testing.T) {
	for _, bits := range []int{16, 24, 32} {
		t.Run(string(rune('0'+bits/8)), func(t *testing.T) {
			width := bits / 8
			data := make([]byte, 2*width)
			data[width-1] = 0x80
			for i := width; i < 2*width; i++ {
				data[i] = 0xff
			}
			data[2*width-1] = 0x7f
			b := make([]byte, 54+len(data))
			copy(b, "RIFF")
			binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
			copy(b[8:], "WAVEJUNK")
			binary.LittleEndian.PutUint32(b[16:], 1)
			b[20] = 42
			copy(b[22:], "fmt ")
			binary.LittleEndian.PutUint32(b[26:], 16)
			binary.LittleEndian.PutUint16(b[30:], 1)
			binary.LittleEndian.PutUint16(b[32:], 1)
			binary.LittleEndian.PutUint32(b[34:], 8000)
			binary.LittleEndian.PutUint32(b[38:], uint32(8000*width))
			binary.LittleEndian.PutUint16(b[42:], uint16(width))
			binary.LittleEndian.PutUint16(b[44:], uint16(bits))
			copy(b[46:], "data")
			binary.LittleEndian.PutUint32(b[50:], uint32(len(data)))
			copy(b[54:], data)
			p := memoryReader{bytes.NewReader(b)}
			w, err := Peaks(context.Background(), p, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(w.Channels[0]) != 2 || w.Channels[0][0].Min != -1 || w.Channels[0][1].Max < .99 {
				t.Fatalf("bad sign extension: %+v", w)
			}
		})
	}
}

type memoryReader struct{ *bytes.Reader }

func (memoryReader) Name() string { return "tone.wav" }
func (memoryReader) Close() error { return nil }
