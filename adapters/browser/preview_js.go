//go:build js && wasm

package browser

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sync"
	"syscall/js"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/resource"
)

// PreviewAvailable reports the actual loaded processing/output host port. The
// main host supplies cross-origin isolation and AudioWorklet support separately.
func PreviewAvailable() bool {
	enabled := js.Global().Get("riffinePreviewEnabled")
	return enabled.Type() == js.TypeBoolean && enabled.Bool() && js.Global().Get("riffinePreviewRun").Type() == js.TypeFunction
}

type Previewer struct{}

func (Previewer) PreviewSource(ctx context.Context, reader resource.Reader, info audio.Info, request engine.PreviewRequest) (engine.PreviewReport, error) {
	var report engine.PreviewReport
	if !PreviewAvailable() {
		return report, errors.New("browser output preview is unavailable")
	}
	pcm, err := (audio.WAVDecoder{}).OpenVerifiedPCM(ctx, reader, info)
	if err != nil {
		return report, err
	}
	defer pcm.Close()
	if err = pcm.SeekFrame(ctx, request.StartFrame); err != nil {
		return report, err
	}
	if request.EndFrame <= request.StartFrame || request.EndFrame > info.Frames {
		return report, errors.New("invalid preview source range")
	}
	remaining := request.EndFrame - request.StartFrame
	samples := make([]float32, 512*info.Channels)
	raw := make([]byte, len(samples)*4)
	var reads sync.WaitGroup
	read := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]
		reads.Add(1)
		go func() {
			defer reads.Done()
			if err := ctx.Err(); err != nil {
				reject.Invoke(err.Error())
				return
			}
			count := int(min(int64(512), remaining))
			if count == 0 {
				reject.Invoke("preview requested source after EOF")
				return
			}
			count, err := pcm.ReadFrames(ctx, samples[:count*info.Channels])
			if err != nil && err != io.EOF {
				reject.Invoke(err.Error())
				return
			}
			if count == 0 {
				reject.Invoke("preview source truncated")
				return
			}
			remaining -= int64(count)
			for i, sample := range samples[:count*info.Channels] {
				binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(sample))
			}
			buffer := js.Global().Get("Uint8Array").New(count * info.Channels * 4)
			js.CopyBytesToJS(buffer, raw[:count*info.Channels*4])
			resolve.Invoke(js.Global().Get("Float32Array").New(buffer.Get("buffer")))
		}()
		return nil
	})
	defer func() { reads.Wait(); read.Release() }()
	type result struct {
		data string
		err  error
	}
	results := make(chan result, 1)
	completed := js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := js.Global().Get("JSON").Call("stringify", args[0]).String()
		select {
		case results <- result{data: data}:
		default:
		}
		return nil
	})
	failed := js.FuncOf(func(_ js.Value, args []js.Value) any {
		select {
		case results <- result{err: errors.New(args[0].String())}:
		default:
		}
		return nil
	})
	defer completed.Release()
	defer failed.Release()
	abort := js.Global().Call("riffinePreviewRun", map[string]any{"channels": info.Channels, "sampleRate": info.SampleRate, "sourceFrames": request.EndFrame - request.StartFrame, "speed": request.Speed, "pitch": request.Pitch}, read, completed, failed)
	var done result
	select {
	case done = <-results:
	case <-ctx.Done():
		abort.Invoke()
		// The JS worker completion is a lifetime fence: it must stop invoking read
		// before this function releases callbacks/cursor and the core closes Reader.
		done = <-results
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if done.err != nil {
		return report, done.err
	}
	if len(done.data) > 4096 {
		return report, errors.New("preview report exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(done.data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	return report, nil
}
