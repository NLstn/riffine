//go:build js && wasm

package browser

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/NLstn/riffine/core/waveform"
)

// PeakCache stores disposable peaks in IndexedDB, independently of projects.
type PeakCache struct{}

func (PeakCache) Get(ctx context.Context, key string, maxBytes int) ([]byte, error) {
	value, err := peakCacheCall(ctx, "get", key, js.Undefined())
	if err != nil {
		return nil, err
	}
	if value.IsNull() || value.Type() != js.TypeObject || value.Get("byteLength").Int() > maxBytes {
		return nil, errors.New("missing or oversized peak cache")
	}
	data := make([]byte, value.Get("byteLength").Int())
	js.CopyBytesToGo(data, value)
	return data, nil
}
func (PeakCache) Put(ctx context.Context, key string, data []byte) error {
	if len(data) > waveform.MaxCacheBytes {
		return errors.New("oversized peak cache")
	}
	value := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(value, data)
	_, err := peakCacheCall(ctx, "put", key, value)
	return err
}
func peakCacheCall(ctx context.Context, method, key string, data js.Value) (js.Value, error) {
	return cacheCall(ctx, "riffinePeakCacheRun", method, key, data)
}
func cacheCall(ctx context.Context, bridge, method, key string, data js.Value) (js.Value, error) {
	if js.Global().Get(bridge).Type() != js.TypeFunction {
		return js.Undefined(), errors.New("analysis storage unavailable")
	}
	return awaitLive(ctx, func(done, failed js.Func) js.Value {
		return js.Global().Call(bridge, method, key, data, done, failed)
	})
}

var _ waveform.Cache = PeakCache{}
