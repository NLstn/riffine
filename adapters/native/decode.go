package native

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"sync"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

// Decoder uses Go's bounded WAV port and the tested native worker for compressed
// local files. It never moves realtime PCM through the Go/UI command boundary.
type Decoder struct {
	adapter Adapter
	formats []string
}

func NewDecoder(ctx context.Context, adapter Adapter) (Decoder, error) {
	var result Decoder
	data, err := adapter.capture(ctx, "formats")
	if err != nil {
		return result, err
	}
	var report struct {
		ABI     int      `json:"abi"`
		Formats []string `json:"formats"`
	}
	if err = json.Unmarshal(data, &report); err != nil {
		return result, err
	}
	if report.ABI != 1 || len(report.Formats) != 3 || report.Formats[0] != "wav-pcm" || report.Formats[1] != "flac" || report.Formats[2] != "mp3" {
		return result, errors.New("native decoder protocol/capabilities are unsupported")
	}
	return Decoder{adapter: adapter, formats: append([]string(nil), report.Formats...)}, nil
}
func (d Decoder) Formats() []string { return append([]string(nil), d.formats...) }
func (a Adapter) capture(ctx context.Context, args ...string) ([]byte, error) {
	if a.Executable == "" {
		return nil, errors.New("native helper path is empty")
	}
	cmd := exec.CommandContext(ctx, a.Executable, args...)
	var output cappedOutput
	output.limit = 4096
	cmd.Stdout = &output
	cmd.Stderr = a.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("audio decoder helper: %w", err)
	}
	return output.Bytes(), nil
}

type cappedOutput struct {
	bytes.Buffer
	limit int
}

func (b *cappedOutput) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.Len() {
		return 0, errors.New("decoder metadata response exceeds limit")
	}
	return b.Buffer.Write(data)
}
func (d Decoder) Inspect(ctx context.Context, r resource.Reader) (audio.Info, error) {
	info, err := (audio.WAVDecoder{}).Inspect(ctx, r)
	if err == nil {
		return info, nil
	}
	if ctx.Err() != nil {
		return audio.Info{}, ctx.Err()
	}
	var header [12]byte
	n, _ := r.ReadAt(header[:], 0)
	if n == 12 && string(header[:4]) == "RIFF" && string(header[8:]) == "WAVE" {
		return audio.Info{}, err
	}
	path, ok := filesystem.LocalPath(r)
	if !ok {
		return audio.Info{}, errors.New("native compressed decoding requires a filesystem-owned resource")
	}
	data, err := d.adapter.capture(ctx, "decoder-info", path)
	if err != nil {
		return audio.Info{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&info); err != nil {
		return audio.Info{}, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return audio.Info{}, errors.New("unexpected decoder metadata")
	}
	if info.Encoding != "flac" && info.Encoding != "mp3" {
		return audio.Info{}, errors.New("native decoder returned an unexpected source codec")
	}
	info.SHA256, err = audio.Fingerprint(ctx, r)
	if err != nil {
		return audio.Info{}, err
	}
	if err = audio.ValidateInfo(info); err != nil {
		return audio.Info{}, err
	}
	return info, nil
}
func (d Decoder) OpenPCM(ctx context.Context, r resource.Reader) (audio.PCMStream, error) {
	info, err := d.Inspect(ctx, r)
	if err != nil {
		return nil, err
	}
	if info.Encoding == "wav-pcm" {
		return (audio.WAVDecoder{}).OpenPCM(ctx, r)
	}
	path, ok := filesystem.LocalPath(r)
	if !ok {
		return nil, errors.New("native compressed decoder requires a local resource")
	}
	return &decodedPCM{adapter: d.adapter, parent: ctx, path: path, info: info, raw: make([]byte, audio.PCMBlockFrames*info.Channels*4)}, nil
}
func (d Decoder) Peaks(ctx context.Context, r resource.Reader, bins int) (audio.Waveform, error) {
	if bins < 1 || bins > 100000 {
		return audio.Waveform{}, errors.New("bins must be 1–100000")
	}
	info, err := d.Inspect(ctx, r)
	if err != nil {
		return audio.Waveform{}, err
	}
	bins = int(min(int64(bins), info.Frames))
	stream, err := d.OpenPCM(ctx, r)
	if err != nil {
		return audio.Waveform{}, err
	}
	defer stream.Close()
	return audio.PeaksFromPCM(ctx, info, stream, bins)
}

// This process adapter is for non-realtime analysis workers. Live native preview
// uses the same decoder C ABI directly, keeping PCM entirely inside C++.
type decodedPCM struct {
	mu             sync.Mutex
	adapter        Adapter
	parent         context.Context
	path           string
	info           audio.Info
	frame          int64
	raw            []byte
	cmd            *exec.Cmd
	pipe           io.ReadCloser
	stderr         cappedOutput
	closed, failed bool
}

func (s *decodedPCM) start() error {
	if s.cmd != nil {
		return nil
	}
	cmd := exec.CommandContext(s.parent, s.adapter.Executable, "decoder-read", s.path, strconv.FormatInt(s.frame, 10))
	s.stderr = cappedOutput{limit: 4096}
	cmd.Stderr = &s.stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		pipe.Close()
		return err
	}
	s.cmd = cmd
	s.pipe = pipe
	return nil
}
func (s *decodedPCM) finish(kill bool) error {
	if s.cmd == nil {
		return nil
	}
	if kill && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.pipe.Close()
	err := s.cmd.Wait()
	s.cmd = nil
	s.pipe = nil
	if !kill && err != nil {
		return fmt.Errorf("PCM decoder: %w: %s", err, s.stderr.String())
	}
	return nil
}
func (s *decodedPCM) ReadFrames(ctx context.Context, dst []float32) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := s.parent.Err(); err != nil {
		return 0, err
	}
	if s.closed || s.failed {
		return 0, errors.New("PCM decoder is closed or failed")
	}
	if len(dst) == 0 || len(dst)%s.info.Channels != 0 || len(dst)/s.info.Channels > audio.PCMBlockFrames {
		return 0, errors.New("PCM buffer must hold 1–8192 complete frames")
	}
	if s.frame == s.info.Frames {
		return 0, io.EOF
	}
	if err := s.start(); err != nil {
		return 0, err
	}
	cmd := s.cmd
	stop := context.AfterFunc(ctx, func() { cmd.Process.Kill() })
	defer stop()
	frames := int(min(int64(len(dst)/s.info.Channels), s.info.Frames-s.frame))
	raw := s.raw[:frames*s.info.Channels*4]
	_, err := io.ReadFull(s.pipe, raw)
	if err != nil || ctx.Err() != nil || s.parent.Err() != nil {
		s.failed = true
		s.finish(true)
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if s.parent.Err() != nil {
			return 0, s.parent.Err()
		}
		return 0, fmt.Errorf("truncated PCM worker output: %w", err)
	}
	if s.frame+int64(frames) == s.info.Frames {
		if err = s.finish(false); err != nil {
			s.failed = true
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			if s.parent.Err() != nil {
				return 0, s.parent.Err()
			}
			return 0, err
		}
	}
	if ctx.Err() != nil {
		s.failed = true
		s.finish(true)
		return 0, ctx.Err()
	}
	if s.parent.Err() != nil {
		s.failed = true
		s.finish(true)
		return 0, s.parent.Err()
	}
	for j := 0; j < frames*s.info.Channels; j++ {
		v := math.Float32frombits(binary.LittleEndian.Uint32(raw[j*4:]))
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			s.failed = true
			s.finish(true)
			return 0, errors.New("non-finite decoded PCM")
		}
		dst[j] = v
	}
	s.frame += int64(frames)
	return frames, nil
}
func (s *decodedPCM) SeekFrame(ctx context.Context, frame int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.parent.Err(); err != nil {
		return err
	}
	if s.closed || s.failed || frame < 0 || frame > s.info.Frames {
		return errors.New("invalid PCM seek")
	}
	s.finish(true)
	s.frame = frame
	return nil
}
func (s *decodedPCM) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.raw = nil
	return s.finish(true)
}

var _ audio.Decoder = Decoder{}
var _ audio.PCMDecoder = Decoder{}

func (a Adapter) inspectSource(ctx context.Context, path string) (audio.Info, error) {
	r, err := (filesystem.Host{}).Open(ctx, path)
	if err != nil {
		return audio.Info{}, err
	}
	defer r.Close()
	if info, err := (audio.WAVDecoder{}).Inspect(ctx, r); err == nil {
		return info, nil
	}
	decoder, err := NewDecoder(ctx, a)
	if err != nil {
		return audio.Info{}, err
	}
	return decoder.Inspect(ctx, r)
}
