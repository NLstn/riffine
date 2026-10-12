package native

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"unicode/utf8"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/resource"
)

const controlMagic uint64 = 0x314350494e464652

// WorkerInfo reports decode/DSP schedule state, never device presentation.
type WorkerInfo struct {
	Channels, SampleRate     int
	Frames                   int64
	Generation, LastSequence uint64
	Pending                  int
}

// ScheduledWorker retains one native helper and two native decoder/DSP slots.
// All calls run on a serialized control worker. No PCM enters this protocol, and
// this is deliberately not an engine.OutputDevice: it cannot acknowledge sound.
// Close/cancel waits for process exit; the caller always owns the original reader.
type ScheduledWorker struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	input     io.WriteCloser
	output    io.ReadCloser
	cancel    context.CancelFunc
	waited    bool
	terminal  error
	requestID uint64
	info      audio.Info
}

func (a Adapter) OpenScheduledWorker(ctx context.Context, reader resource.Reader, info audio.Info) (*ScheduledWorker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := audio.ValidateInfo(info); err != nil {
		return nil, err
	}
	decoder, err := NewDecoder(ctx, a)
	if err != nil {
		return nil, err
	}
	if decoder.scheduledControl != 1 {
		return nil, errors.New("native scheduled control is unsupported; rebuild the helper")
	}
	// The request context bounds opening/handshake, not the worker lifetime.
	lifetime, cancel := context.WithCancel(context.Background())
	cmd, release, err := a.sourceCommand(lifetime, reader, "scheduled-control-handle")
	if err != nil {
		cancel()
		return nil, err
	}
	defer release()
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	cmd.Stderr = a.Stderr
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		cancel()
		return nil, err
	}
	worker := &ScheduledWorker{cmd: cmd, input: input, output: output, cancel: cancel, info: info}
	worker.mu.Lock()
	_, err = worker.exchangeLocked(ctx, [16]uint64{3: 1})
	worker.mu.Unlock()
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		worker.Close()
		return nil, err
	}
	return worker, nil
}

func (w *ScheduledWorker) shutdownLocked(cause error) {
	if w.terminal == nil {
		w.terminal = cause
	}
	w.cancel()
	w.input.Close()
	if !w.waited {
		_ = w.cmd.Wait()
		w.waited = true
	}
	w.output.Close()
}
func (w *ScheduledWorker) exchangeLocked(ctx context.Context, q [16]uint64) (WorkerInfo, error) {
	var info WorkerInfo
	if err := ctx.Err(); err != nil {
		return info, err
	}
	if w.terminal != nil {
		return info, w.terminal
	}
	if w.requestID == math.MaxUint64 {
		err := errors.New("native control request sequence exhausted")
		w.shutdownLocked(err)
		return info, err
	}
	w.requestID++
	q[0], q[1], q[2] = controlMagic, 1, w.requestID
	var request [128]byte
	for i, word := range q {
		binary.LittleEndian.PutUint64(request[i*8:], word)
	}
	stop := context.AfterFunc(ctx, w.cancel)
	defer stop()
	var reply [640]byte
	written, err := w.input.Write(request[:])
	if err == nil && written != len(request) {
		err = io.ErrShortWrite
	}
	if err == nil {
		_, err = io.ReadFull(w.output, reply[:])
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		w.shutdownLocked(err)
		return info, err
	}
	var words [16]uint64
	for i := range words {
		words[i] = binary.LittleEndian.Uint64(reply[i*8:])
	}
	protocolError := func(message string) (WorkerInfo, error) {
		err := errors.New(message)
		w.shutdownLocked(err)
		return WorkerInfo{}, err
	}
	if words[0] != controlMagic || words[1] != 1 || words[2] != q[2] || words[3] > 1 {
		return protocolError("invalid native control response envelope")
	}
	for _, word := range words[10:] {
		if word != 0 {
			return protocolError("unexpected native control response fields")
		}
	}
	if words[4] != uint64(w.info.Channels) || words[5] != uint64(w.info.SampleRate) || words[6] != uint64(w.info.Frames) || words[9] > 2 {
		return protocolError("native control source dimensions or slot bounds changed")
	}
	info = WorkerInfo{Channels: int(words[4]), SampleRate: int(words[5]), Frames: int64(words[6]), Generation: words[7], LastSequence: words[8], Pending: int(words[9])}
	message := reply[128:]
	end := 0
	for end < len(message) && message[end] != 0 {
		end++
	}
	if end == len(message) || !utf8.Valid(message[:end]) {
		return protocolError("invalid native control error text")
	}
	for _, b := range message[end:] {
		if b != 0 {
			return protocolError("trailing native control error data")
		}
	}
	if words[3] != 0 {
		if end == 0 {
			return protocolError("empty native control failure")
		}
		return info, fmt.Errorf("native scheduled worker: %s", message[:end])
	}
	if end != 0 {
		return protocolError("unexpected native control error")
	}
	if (q[3] == 2 && (info.Generation != q[4] || info.Pending != 0)) || (q[3] == 3 && (info.Generation != q[4] || info.LastSequence != q[5] || info.Pending < 1)) {
		return protocolError("native control schedule/reset result is inconsistent")
	}
	return info, nil
}
func (w *ScheduledWorker) Info(ctx context.Context) (WorkerInfo, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.exchangeLocked(ctx, [16]uint64{3: 1})
}
func (w *ScheduledWorker) Reset(ctx context.Context, generation uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.exchangeLocked(ctx, [16]uint64{3: 2, 4: generation})
	return err
}
func (w *ScheduledWorker) Submit(ctx context.Context, s engine.Schedule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if s.StartFrame < 0 || s.EndFrame < 0 || s.OutputStart < 0 || s.OutputEnd < 0 || s.EdgeFadeFrames < 0 {
		return errors.New("negative native schedule coordinate")
	}
	_, err := w.exchangeLocked(ctx, [16]uint64{3: 3, 4: s.Generation, 5: s.Sequence, 6: uint64(s.StartFrame), 7: uint64(s.EndFrame), 8: uint64(s.OutputStart), 9: uint64(s.OutputEnd), 10: uint64(s.EdgeFadeFrames), 11: math.Float64bits(s.Speed), 12: math.Float64bits(s.Pitch)})
	return err
}
func (w *ScheduledWorker) Close() error {
	// Cancel before taking the mutex so teardown also interrupts stalled pipes.
	w.cancel()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shutdownLocked(errors.New("native scheduled worker is closed"))
	return nil
}
