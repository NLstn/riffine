package audio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestToneAndPeaks(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tone.wav")
	if err := WriteTone(p, 1, 440, 48000, 2); err != nil {
		t.Fatal(err)
	}
	i, err := Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	if i.Frames != 48000 || i.Channels != 2 || i.Duration() != 1 || len(i.SHA256) != 64 {
		t.Fatalf("bad metadata: %+v", i)
	}
	w, err := Peaks(p, 17)
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
	if err = WriteTone(p, 1, 440, 48000, 1); err == nil {
		t.Fatal("overwrote existing file")
	}
	if _, err = Peaks(p, 0); err == nil {
		t.Fatal("accepted zero bins")
	}
}

func TestMalformedWAV(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.wav")
	if err := WriteTone(base, .1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(base)
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
			p := filepath.Join(dir, "bad.wav")
			b := mutate(append([]byte(nil), original...))
			if err := os.WriteFile(p, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Inspect(p); err == nil {
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
			p := filepath.Join(t.TempDir(), "depth.wav")
			os.WriteFile(p, b, 0600)
			w, err := Peaks(p, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(w.Channels[0]) != 2 || w.Channels[0][0].Min != -1 || w.Channels[0][1].Max < .99 {
				t.Fatalf("bad sign extension: %+v", w)
			}
		})
	}
}
