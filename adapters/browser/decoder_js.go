//go:build js && wasm

package browser

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
	"syscall/js"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

// Decoder keeps original compressed identity and source-frame coordinates.
// A separate cancellable worker owns each bounded decoding cursor.
type Decoder struct{}

func (Decoder) Formats() []string {
	if js.Global().Get("riffineCodecOpen").Type() == js.TypeFunction {
		return []string{"wav-pcm", "mp3", "flac", "aac", "alac", "opus"}
	}
	return []string{"wav-pcm"}
}
func sourceCodec(r resource.Reader) (int, error) {
	var header [12]byte
	if _, err := r.ReadAt(header[:], 0); err != nil {
		return 0, err
	}
	if string(header[:4]) == "RIFF" {
		return 0, nil
	}
	if string(header[:4]) == "fLaC" {
		return 2, nil
	}
	if string(header[:3]) == "ID3" || (header[0] == 255 && header[1]&224 == 224) {
		return 1, nil
	}
	if string(header[4:8]) == "ftyp" {
		return 3, nil
	}
	if string(header[:4]) == "OggS" {
		return 4, nil
	}
	return 0, errors.New("unsupported practice audio: choose PCM WAV, MP3, FLAC, M4A (AAC/Apple Lossless) or Opus")
}
func (d Decoder) Inspect(ctx context.Context, r resource.Reader) (audio.Info, error) {
	if err := ctx.Err(); err != nil {
		return audio.Info{}, err
	}
	reader, immutable := r.(*fileReader)
	if immutable {
		reader.mu.Lock()
		closed := reader.closed
		reader.mu.Unlock()
		if closed {
			return audio.Info{}, errors.New("selected file reader is closed")
		}
		reader.owner.mu.Lock()
		cached := reader.owner.inspection
		reader.owner.mu.Unlock()
		if cached != nil {
			return *cached, nil
		}
	}
	info, err := d.inspect(ctx, r)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && immutable {
		reader.owner.mu.Lock()
		reader.owner.inspection = &info
		reader.owner.mu.Unlock()
	}
	return info, err
}
func (d Decoder) inspect(ctx context.Context, r resource.Reader) (audio.Info, error) {
	codec, err := sourceCodec(r)
	if err != nil {
		return audio.Info{}, err
	}
	if codec == 0 {
		return (audio.WAVDecoder{}).Inspect(ctx, r)
	}
	stream, err := openCompressed(ctx, r, codec)
	if err != nil {
		return audio.Info{}, err
	}
	info := stream.info
	if err = stream.Close(); err != nil {
		return audio.Info{}, err
	}
	info.SHA256, err = audio.Fingerprint(ctx, r)
	if err != nil {
		return audio.Info{}, err
	}
	return info, audio.ValidateInfo(info)
}
func (d Decoder) OpenPCM(ctx context.Context, r resource.Reader) (audio.PCMStream, error) {
	info, err := d.Inspect(ctx, r)
	if err != nil {
		return nil, err
	}
	return d.OpenVerifiedPCM(ctx, r, info)
}
func (d Decoder) OpenVerifiedPCM(ctx context.Context, r resource.Reader, info audio.Info) (audio.PCMStream, error) {
	if err := audio.ValidateInfo(info); err != nil {
		return nil, err
	}
	if info.Encoding == "wav-pcm" {
		return (audio.WAVDecoder{}).OpenVerifiedPCM(ctx, r, info)
	}
	var codec int
	switch info.Encoding {
	case "mp3":
		codec = 1
	case "flac":
		codec = 2
	case "aac", "alac":
		codec = 3
	case "opus":
		codec = 4
	default:
		return nil, errors.New("unsupported source codec")
	}
	stream, err := openCompressed(ctx, r, codec)
	if err != nil {
		return nil, err
	}
	actual := stream.info
	if actual.Encoding != info.Encoding || actual.SampleRate != info.SampleRate || actual.Channels != info.Channels || actual.Frames != info.Frames {
		stream.Close()
		return nil, errors.New("compressed source dimensions changed")
	}
	return stream, nil
}
func (d Decoder) Peaks(ctx context.Context, r resource.Reader, bins int) (audio.Waveform, error) {
	info, err := d.Inspect(ctx, r)
	if err != nil {
		return audio.Waveform{}, err
	}
	stream, err := d.OpenVerifiedPCM(ctx, r, info)
	if err != nil {
		return audio.Waveform{}, err
	}
	defer stream.Close()
	return audio.PeaksFromPCM(ctx, info, stream, bins)
}

type compressedPCM struct {
	mu     sync.Mutex
	port   js.Value
	info   audio.Info
	frame  int64
	closed bool
	raw    [8192 * 2 * 4]byte
}

func openCompressed(ctx context.Context, r resource.Reader, codec int) (*compressedPCM, error) {
	reader, ok := r.(*fileReader)
	if !ok {
		return nil, errors.New("compressed decoder requires the borrowed selected File")
	}
	reader.mu.Lock()
	file := reader.file
	closed := reader.closed
	reader.mu.Unlock()
	if closed || file.IsUndefined() || js.Global().Get("riffineCodecOpen").Type() != js.TypeFunction {
		return nil, errors.New("compressed decoding is unavailable; build the browser processing assets")
	}
	value, err := awaitLive(ctx, func(done, failed js.Func) js.Value {
		return js.Global().Call("riffineCodecOpen", file, codec, done, failed)
	})
	if err != nil {
		return nil, err
	}
	meta := value.Get("info")
	info := audio.Info{Encoding: meta.Get("encoding").String(), Channels: meta.Get("channels").Int(), SampleRate: meta.Get("sampleRate").Int(), Frames: int64(meta.Get("frames").Float()), Bits: meta.Get("bits").Int()}
	stream := &compressedPCM{port: value.Get("port"), info: info}
	if codec == 2 {
		var header [26]byte
		if _, readErr := r.ReadAt(header[:], 0); readErr != nil {
			stream.Close()
			return nil, readErr
		}
		info.Bits = int(((header[20]&1)<<4)|(header[21]>>4)) + 1
	}
	stream.info = info
	// Canonical hash is computed from the original resource by Inspect/core, not decoder PCM.
	check := info
	check.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err = audio.ValidateInfo(check); err != nil {
		stream.Close()
		return nil, err
	}
	return stream, nil
}
func (s *compressedPCM) call(ctx context.Context, method string, args any) (js.Value, error) {
	return awaitLive(ctx, func(done, failed js.Func) js.Value { return s.port.Call("call", method, args, done, failed) })
}
func (s *compressedPCM) ReadFrames(ctx context.Context, dst []float32) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if s.closed {
		return 0, errors.New("PCM stream closed")
	}
	channels := s.info.Channels
	if len(dst) == 0 || len(dst)%channels != 0 || len(dst)/channels > audio.PCMBlockFrames {
		return 0, errors.New("PCM buffer requires 1–8192 complete frames")
	}
	if s.frame == s.info.Frames {
		return 0, io.EOF
	}
	value, err := s.call(ctx, "read", map[string]any{"count": len(dst) / channels})
	if err != nil {
		return 0, err
	}
	n := value.Get("frames").Int()
	pcm := value.Get("pcm")
	if n < 1 || n > len(dst)/channels || int64(n) > s.info.Frames-s.frame || pcm.Get("byteLength").Int() != n*channels*4 {
		return 0, errors.New("invalid or truncated decoder PCM")
	}
	raw := s.raw[:n*channels*4]
	if js.CopyBytesToGo(raw, pcm) != len(raw) {
		return 0, io.ErrUnexpectedEOF
	}
	for i := 0; i < n*channels; i++ {
		dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	s.frame += int64(n)
	return n, nil
}
func (s *compressedPCM) SeekFrame(ctx context.Context, frame int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return errors.New("PCM stream closed")
	}
	if frame < 0 || frame > s.info.Frames {
		return errors.New("PCM seek exceeds source range")
	}
	if frame == s.frame {
		return nil
	}
	if _, err := s.call(ctx, "seek", map[string]any{"position": float64(frame)}); err != nil {
		return err
	}
	s.frame = frame
	return nil
}
func (s *compressedPCM) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	_, err := s.call(context.Background(), "close", map[string]any{})
	return err
}

var _ audio.Decoder = Decoder{}
var _ audio.PCMDecoder = Decoder{}
