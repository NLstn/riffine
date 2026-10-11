package waveform

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"math"

	"github.com/NLstn/riffine/core/audio"
)

func validateInfo(i audio.Info) error {
	hash, err := hex.DecodeString(i.SHA256)
	if err != nil || len(hash) != 32 || i.SampleRate < 8000 || i.SampleRate > 192000 || i.Channels < 1 || i.Channels > 2 || i.Frames < 1 || i.Frames > 1<<48 {
		return errors.New("invalid waveform source metadata")
	}
	return nil
}

// The header and body each carry a checksum. Counts are derived from metadata,
// not trusted cache values, before allocation. Coordinates use 64-bit frames.
func encode(p *Pyramid) []byte {
	var b bytes.Buffer
	b.WriteString("RIFFWV1\x00")
	put := func(v any) {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			panic(err)
		}
	}
	put(uint32(AlgorithmVersion))
	put(uint32(p.info.SampleRate))
	put(uint32(p.info.Channels))
	put(uint64(p.info.Frames))
	put(uint64(p.base))
	hash, _ := hex.DecodeString(p.info.SHA256)
	b.Write(hash)
	put(uint32(len(p.levels)))
	// Reserve body and header checksums.
	put(uint32(0))
	put(uint32(0))
	header := append([]byte(nil), b.Bytes()...)
	b.Reset()
	for _, level := range p.levels {
		put(uint32(len(level[0])))
		for _, channel := range level {
			for _, peak := range channel {
				put(math.Float32bits(peak.Min))
				put(math.Float32bits(peak.Max))
			}
		}
	}
	body := b.Bytes()
	binary.LittleEndian.PutUint32(header[72:76], crc32.ChecksumIEEE(body))
	binary.LittleEndian.PutUint32(header[76:80], crc32.ChecksumIEEE(header[:76]))
	return append(header, body...)
}
func decode(data []byte, info audio.Info) (*Pyramid, error) {
	bad := func() (*Pyramid, error) { return nil, errors.New("invalid, corrupt or incompatible waveform cache") }
	if len(data) < 80 || len(data) > MaxCacheBytes || string(data[:8]) != "RIFFWV1\x00" || binary.LittleEndian.Uint32(data[8:12]) != AlgorithmVersion {
		return bad()
	}
	if crc32.ChecksumIEEE(data[:76]) != binary.LittleEndian.Uint32(data[76:80]) || crc32.ChecksumIEEE(data[80:]) != binary.LittleEndian.Uint32(data[72:76]) {
		return bad()
	}
	if int(binary.LittleEndian.Uint32(data[12:16])) != info.SampleRate || int(binary.LittleEndian.Uint32(data[16:20])) != info.Channels || binary.LittleEndian.Uint64(data[20:28]) != uint64(info.Frames) || binary.LittleEndian.Uint64(data[28:36]) != uint64(baseFrames(info.Frames)) || hex.EncodeToString(data[36:68]) != info.SHA256 {
		return bad()
	}
	levels := binary.LittleEndian.Uint32(data[68:72])
	if levels < 1 || levels > 17 {
		return bad()
	}
	p := &Pyramid{info: info, base: baseFrames(info.Frames)}
	count := int((info.Frames + p.base - 1) / p.base)
	offset := 80
	for l := uint32(0); l < levels; l++ {
		needed := 4 + count*info.Channels*8
		if needed > len(data)-offset || binary.LittleEndian.Uint32(data[offset:offset+4]) != uint32(count) {
			return bad()
		}
		offset += 4
		level := make([][]audio.Peak, info.Channels)
		for c := range level {
			level[c] = make([]audio.Peak, count)
			for j := range level[c] {
				a := math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
				b := math.Float32frombits(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
				offset += 8
				if math.IsNaN(float64(a)) || math.IsInf(float64(a), 0) || math.IsNaN(float64(b)) || math.IsInf(float64(b), 0) || a > b {
					return bad()
				}
				level[c][j] = audio.Peak{Min: a, Max: b}
			}
		}
		p.levels = append(p.levels, level)
		if count == 1 {
			if l != levels-1 {
				return bad()
			}
		} else {
			if l == levels-1 {
				return bad()
			}
			count = (count + 1) / 2
		}
	}
	if offset != len(data) {
		return bad()
	}
	// A forged checksum still cannot change aggregate levels silently.
	for l := 1; l < len(p.levels); l++ {
		for c := 0; c < info.Channels; c++ {
			for j, actual := range p.levels[l][c] {
				previous := p.levels[l-1][c]
				expected := previous[2*j]
				if 2*j+1 < len(previous) {
					b := previous[2*j+1]
					expected.Min = min(expected.Min, b.Min)
					expected.Max = max(expected.Max, b.Max)
				}
				if actual != expected {
					return bad()
				}
			}
		}
	}
	return p, nil
}
