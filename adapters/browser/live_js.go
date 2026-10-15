//go:build js && wasm

package browser

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"
	"syscall/js"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/practice"
)

func LiveAvailable() bool {
	enabled := js.Global().Get("riffineLiveEnabled")
	return enabled.Type() == js.TypeBoolean && enabled.Bool() && js.Global().Get("riffineLiveOpen").Type() == js.TypeFunction
}

type LiveFactory struct{}
type liveDevice struct {
	info       audio.Info
	pcm        audio.PCMStream
	control    js.Value
	read       js.Func
	readClicks js.Func
	scheduleMu sync.Mutex
	schedules  map[uint64]engine.Schedule
	reads      sync.WaitGroup
	readMu     sync.Mutex
	samples    [1024]float32
	raw        [4096]byte
	lifetime   context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
	closeErr   error
}
type liveResult struct {
	value js.Value
	err   error
}

// Context controls this operation only. On cancellation the JS abort/completion
// fence must finish before callbacks or the verified source reader are released.
func awaitLive(ctx context.Context, invoke func(js.Func, js.Func) js.Value) (js.Value, error) {
	results := make(chan liveResult, 1)
	complete := js.FuncOf(func(_ js.Value, args []js.Value) any {
		select {
		case results <- liveResult{value: args[0]}:
		default:
		}
		return nil
	})
	failed := js.FuncOf(func(_ js.Value, args []js.Value) any {
		select {
		case results <- liveResult{err: errors.New(args[0].String())}:
		default:
		}
		return nil
	})
	defer complete.Release()
	defer failed.Release()
	abort := invoke(complete, failed)
	var result liveResult
	var cancellation error
	fenceAbort := func(waitResult bool) {
		fences := make(chan string, 1)
		fence := js.FuncOf(func(_ js.Value, args []js.Value) any { fences <- args[0].String(); return nil })
		abort.Invoke(fence)
		if waitResult {
			result = <-results
		}
		closeError := <-fences
		fence.Release()
		cancellation = ctx.Err()
		if closeError != "" {
			cancellation = errors.Join(cancellation, errors.New(closeError))
		}
	}
	select {
	case result = <-results:
		// Completion and cancellation can race. A discarded successful open still
		// owns callbacks until the abort fence has closed the returned JS device.
		if ctx.Err() != nil {
			fenceAbort(false)
		}
	case <-ctx.Done():
		fenceAbort(true)
	}
	if cancellation != nil {
		return js.Undefined(), cancellation
	}
	return result.value, result.err
}

func (LiveFactory) OpenLive(ctx context.Context, source practice.VerifiedSource) (practice.LiveDevice, error) {
	if !LiveAvailable() {
		return nil, errors.New("browser live output is unavailable")
	}
	pcm, err := (Decoder{}).OpenVerifiedPCM(ctx, source.Reader, source.Info)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	d := &liveDevice{info: source.Info, pcm: pcm, lifetime: lifetime, cancel: cancel, control: js.Undefined(), schedules: map[uint64]engine.Schedule{}}
	d.read = js.FuncOf(func(_ js.Value, args []js.Value) any {
		start, count, resolve, reject := args[0].Float(), args[1].Float(), args[2], args[3]
		d.reads.Add(1)
		go func() {
			defer d.reads.Done()
			d.readMu.Lock()
			defer d.readMu.Unlock()
			if math.IsNaN(start) || math.IsInf(start, 0) || math.Trunc(start) != start || start < 0 || start > float64(d.info.Frames) || math.IsNaN(count) || math.IsInf(count, 0) || math.Trunc(count) != count || count < 1 || count > 512 || start+count > float64(d.info.Frames) {
				reject.Invoke("invalid scheduled source read")
				return
			}
			if err := d.pcm.SeekFrame(d.lifetime, int64(start)); err != nil {
				reject.Invoke(err.Error())
				return
			}
			samples := d.samples[:int(count)*d.info.Channels]
			n, err := d.pcm.ReadFrames(d.lifetime, samples)
			if err != nil && err != io.EOF {
				reject.Invoke(err.Error())
				return
			}
			if n != int(count) {
				reject.Invoke("scheduled source truncated")
				return
			}
			raw := d.raw[:len(samples)*4]
			for i, sample := range samples {
				binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(sample))
			}
			buffer := js.Global().Get("Uint8Array").New(len(raw))
			js.CopyBytesToJS(buffer, raw)
			resolve.Invoke(js.Global().Get("Float32Array").New(buffer.Get("buffer")))
		}()
		return nil
	})
	d.readClicks = js.FuncOf(func(_ js.Value, args []js.Value) any {
		generation, sequence, start, count := args[0].String(), args[1].String(), args[2].Float(), args[3].Float()
		resolve, reject := args[4], args[5]
		d.reads.Add(1)
		go func() {
			defer d.reads.Done()
			g, err := strconv.ParseUint(generation, 10, 64)
			seq, seqErr := strconv.ParseUint(sequence, 10, 64)
			if err != nil || seqErr != nil || strconv.FormatUint(g, 10) != generation || strconv.FormatUint(seq, 10) != sequence || !validLiveInteger(start) || !validLiveInteger(count) || count < 1 || count > 8192 {
				reject.Invoke("invalid bounded beat window")
				return
			}
			d.scheduleMu.Lock()
			schedule, ok := d.schedules[seq]
			d.scheduleMu.Unlock()
			if !ok || schedule.Generation != g || start+count > float64(schedule.OutputEnd-schedule.OutputStart) {
				reject.Invoke("beat window references unavailable schedule")
				return
			}
			clicks, err := schedule.ClickWindow(d.lifetime, max(0, int64(start)-int64(d.info.SampleRate/100)), int64(start+count))
			if err != nil {
				reject.Invoke(err.Error())
				return
			}
			if clicks == nil {
				resolve.Invoke(js.Global().Get("Array").New())
				return
			}
			data, err := json.Marshal(clicks)
			if err != nil {
				reject.Invoke(err.Error())
				return
			}
			resolve.Invoke(js.Global().Get("JSON").Call("parse", string(data)))
		}()
		return nil
	})
	value, err := awaitLive(ctx, func(complete, failed js.Func) js.Value {
		return js.Global().Call("riffineLiveOpen", map[string]any{"source": source.Handle, "channels": source.Info.Channels, "sampleRate": source.Info.SampleRate, "frames": source.Info.Frames}, d.read, d.readClicks, complete, failed)
	})
	if err != nil {
		cancel()
		d.reads.Wait()
		d.read.Release()
		d.readClicks.Release()
		return nil, errors.Join(err, pcm.Close())
	}
	d.control = value
	return d, nil
}
func validLiveInteger(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value && value >= 0 && value <= 1<<53-1
}
func (d *liveDevice) SupportsCues() bool {
	return d.control.Get("cuesSupported").Type() == js.TypeBoolean && d.control.Get("cuesSupported").Bool()
}
func (d *liveDevice) SupportsMetronome() bool {
	return d.control.Get("metronomeSupported").Type() == js.TypeBoolean && d.control.Get("metronomeSupported").Bool()
}
func (d *liveDevice) OutputSampleRate() int { return d.info.SampleRate }
func (d *liveDevice) call(ctx context.Context, method string, args any) (string, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	value, err := awaitLive(ctx, func(complete, failed js.Func) js.Value {
		return d.control.Call("call", method, string(data), complete, failed)
	})
	if err != nil {
		return "", err
	}
	text := value.String()
	if len(text) > 16384 {
		return "", errors.New("live device response exceeds bound")
	}
	return text, nil
}
func (d *liveDevice) Reset(ctx context.Context, generation uint64) error {
	_, err := d.call(ctx, "reset", map[string]any{"generation": strconv.FormatUint(generation, 10)})
	if err == nil {
		d.scheduleMu.Lock()
		d.schedules = map[uint64]engine.Schedule{}
		d.scheduleMu.Unlock()
	}
	return err
}
func (d *liveDevice) Schedule(ctx context.Context, s engine.Schedule) error {
	d.scheduleMu.Lock()
	d.schedules[s.Sequence] = s
	d.scheduleMu.Unlock()
	_, err := d.call(ctx, "schedule", map[string]any{"generation": strconv.FormatUint(s.Generation, 10), "sequence": strconv.FormatUint(s.Sequence, 10), "source": s.Source, "startFrame": s.StartFrame, "endFrame": s.EndFrame, "outputStart": s.OutputStart, "outputEnd": s.OutputEnd, "speed": s.Speed, "pitch": s.Pitch, "edgeFadeFrames": s.EdgeFadeFrames, "phase": s.Phase, "cueOffsetFrames": s.CueOffsetFrames, "clicks": s.Clicks, "clickLevel": s.ClickLevel, "metronome": s.BeatGrid != nil})
	d.scheduleMu.Lock()
	if err != nil {
		delete(d.schedules, s.Sequence)
	} else {
		for len(d.schedules) > 2 {
			oldest := s.Sequence
			for seq := range d.schedules {
				oldest = min(oldest, seq)
			}
			delete(d.schedules, oldest)
		}
	}
	d.scheduleMu.Unlock()
	return err
}
func (d *liveDevice) Start(ctx context.Context) error {
	_, err := d.call(ctx, "start", struct{}{})
	return err
}

func (d *liveDevice) ConfigureParameters(ctx context.Context, sequence uint64, speed, pitch float64) (engine.ParameterChange, error) {
	text, err := d.call(ctx, "configure", map[string]any{"sequence": strconv.FormatUint(sequence, 10), "speed": speed, "pitch": pitch})
	var wire struct {
		Applied          bool            `json:"applied"`
		Sequence         string          `json:"sequence"`
		SourceFrame      int64           `json:"sourceFrame"`
		OutputFrame      int64           `json:"outputFrame"`
		OutputEnd        int64           `json:"outputEnd"`
		Acknowledgements json.RawMessage `json:"acknowledgements"`
	}
	if err != nil {
		return engine.ParameterChange{}, err
	}
	if err = json.Unmarshal([]byte(text), &wire); err != nil {
		return engine.ParameterChange{}, err
	}
	change := engine.ParameterChange{Applied: wire.Applied, SourceFrame: wire.SourceFrame, OutputFrame: wire.OutputFrame, OutputEnd: wire.OutputEnd}
	if change.Acknowledgements, err = decodeLiveAcks(string(wire.Acknowledgements), 128); err != nil {
		return change, err
	}
	if !wire.Applied {
		return change, nil
	}
	if change.Sequence, err = strconv.ParseUint(wire.Sequence, 10, 64); err != nil {
		return change, err
	}
	d.scheduleMu.Lock()
	defer d.scheduleMu.Unlock()
	old, ok := d.schedules[change.Sequence]
	if !ok {
		return change, errors.New("parameter update references unknown schedule")
	}
	prefix := old
	prefix.EndFrame = change.SourceFrame
	prefix.OutputEnd = change.OutputFrame
	prefix.EdgeFadeFrames = 0
	tail := old
	tail.Sequence = sequence
	tail.StartFrame = change.SourceFrame
	tail.OutputStart = change.OutputFrame
	tail.OutputEnd = change.OutputEnd
	tail.Speed = speed
	tail.Pitch = pitch
	tail.EdgeFadeFrames = 0
	d.schedules = map[uint64]engine.Schedule{prefix.Sequence: prefix, tail.Sequence: tail}
	return change, nil
}

func (d *liveDevice) Suspend(ctx context.Context) ([]engine.OutputAcknowledgement, error) {
	text, err := d.call(ctx, "suspend", struct{}{})
	if err != nil {
		return nil, err
	}
	return decodeLiveAcks(text, 128)
}
func (d *liveDevice) DrainAcknowledgements(ctx context.Context, limit int) ([]engine.OutputAcknowledgement, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 128 {
		return nil, fmt.Errorf("invalid live acknowledgement limit %d", limit)
	}
	value := d.control.Call("drain", limit)
	if message := value.Get("error"); message.Type() == js.TypeString && message.String() != "" {
		return nil, errors.New(message.String())
	}
	return decodeLiveAcks(value.Get("data").String(), limit)
}
func (d *liveDevice) Close() error {
	d.closeOnce.Do(func() {
		d.cancel()
		_, err := d.call(context.Background(), "close", struct{}{})
		d.reads.Wait()
		d.read.Release()
		d.readClicks.Release()
		d.closeErr = errors.Join(err, d.pcm.Close())
	})
	return d.closeErr
}
