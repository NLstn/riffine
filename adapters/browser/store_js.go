//go:build js && wasm

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"syscall/js"

	"github.com/NLstn/riffine/core/resource"
)

// Selected downloads remain read-only; browser: handles address the host's
// IndexedDB document store. Shared core owns encoding and expected revisions.
func (f *Files) storeCall(ctx context.Context, method, handle, revision string, data []byte) (js.Value, error) {
	if js.Global().Get("riffineProjectStoreRun").Type() != js.TypeFunction {
		return js.Undefined(), errors.New("device storage is unavailable; download the project")
	}
	request, err := json.Marshal(map[string]string{"method": method, "handle": handle, "revision": revision})
	if err != nil {
		return js.Undefined(), err
	}
	value, err := awaitLive(ctx, func(done, failed js.Func) js.Value {
		return js.Global().Call("riffineProjectStoreRun", string(request), string(data), done, failed)
	})
	if err != nil && strings.HasPrefix(err.Error(), "revision_conflict:") {
		err = errors.Join(resource.ErrRevisionConflict, err)
	}
	return value, err
}
func (f *Files) Update(ctx context.Context, document, revision string, data []byte) (resource.UpdateResult, error) {
	if !strings.HasPrefix(document, "browser:") {
		return resource.UpdateResult{}, errors.New("downloaded projects are read-only; save a new device copy")
	}
	value, err := f.storeCall(ctx, "update", document, revision, data)
	if err != nil {
		return resource.UpdateResult{}, err
	}
	return resource.UpdateResult{Revision: value.Get("revision").String(), Recovery: value.Get("recovery").String()}, nil
}
