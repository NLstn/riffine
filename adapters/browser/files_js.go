//go:build js && wasm

// Package browser adapts immutable selected File objects to shared core ports.
// FileReaderSync is used only on a dedicated worker, never the UI/audio thread.
package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
)

type Files struct {
	file     js.Value
	document []byte
	mu       sync.Mutex
	// A selected File is immutable. Keep only its successful original-byte
	// inspection; each new selection gets a separate Files owner.
	inspection *audio.Info
}

func NewFiles(file js.Value, document []byte) (*Files, error) {
	if file.Type() != js.TypeObject || file.IsNull() || file.Get("size").Type() != js.TypeNumber || file.Get("name").Type() != js.TypeString || js.Global().Get("FileReaderSync").Type() != js.TypeFunction {
		return nil, errors.New("a selected File and worker FileReaderSync are required")
	}
	size := file.Get("size").Float()
	if math.IsNaN(size) || math.IsInf(size, 0) || math.Trunc(size) != size || size < 1 || size > 1<<48 || len(document) > project.MaxDocumentBytes {
		return nil, errors.New("source size or project exceeds limits")
	}
	return &Files{file: file, document: bytes.Clone(document)}, nil
}
func (f *Files) Open(ctx context.Context, handle string) (resource.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if handle != "source" {
		return nil, errors.New("source handle is unavailable")
	}
	return &fileReader{owner: f, file: f.file, reader: js.Global().Get("FileReaderSync").New(), size: int64(f.file.Get("size").Float()), name: f.file.Get("name").String(), nextYield: time.Now().Add(8 * time.Millisecond)}, nil
}
func (f *Files) Resolve(ctx context.Context, document, reference string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "source", nil
}
func (f *Files) Reference(ctx context.Context, document, handle string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if handle != "source" {
		return "", errors.New("invalid source handle")
	}
	return f.file.Get("name").String(), nil
}
func (f *Files) Read(ctx context.Context, document string, max int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.HasPrefix(document, "browser:") {
		value, err := f.storeCall(ctx, "read", document, "", nil)
		if err != nil {
			return nil, err
		}
		data := []byte(value.Get("document").String())
		if len(data) == 0 || len(data) > max {
			return nil, errors.New("project missing or oversized")
		}
		return data, nil
	}
	if len(f.document) == 0 || len(f.document) > max {
		return nil, errors.New("project missing or oversized")
	}
	return bytes.Clone(f.document), nil
}
func (f *Files) Create(ctx context.Context, document string, data []byte) error {
	if !strings.HasPrefix(document, "browser:") {
		return errors.New("downloaded projects are read-only; save a device copy")
	}
	_, err := f.storeCall(ctx, "create", document, "", data)
	return err
}

type fileReader struct {
	mu           sync.Mutex
	owner        *Files
	file, reader js.Value
	size         int64
	name         string
	closed       bool
	nextYield    time.Time
}

func (r *fileReader) Name() string { return r.name }
func (r *fileReader) Size() int64  { return r.size }
func (r *fileReader) ReadAt(dst []byte, offset int64) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("browser file read: %v", failure)
		}
	}()
	if r.closed {
		return 0, errors.New("selected file reader is closed")
	}
	if offset < 0 {
		return 0, errors.New("negative file offset")
	}
	if len(dst) == 0 {
		return 0, nil
	}
	if offset >= r.size {
		return 0, io.EOF
	}
	for n < len(dst) && offset+int64(n) < r.size {
		// Bound uninterrupted work, rather than paying a browser timer (often
		// clamped to several milliseconds) for every small fingerprint/PCM read.
		// This still lets cancellation messages run between bounded source reads.
		if !time.Now().Before(r.nextYield) {
			time.Sleep(time.Millisecond)
			r.nextYield = time.Now().Add(8 * time.Millisecond)
		}
		end := min(offset+int64(n)+int64(min(len(dst)-n, 64<<10)), r.size)
		buffer := r.reader.Call("readAsArrayBuffer", r.file.Call("slice", float64(offset+int64(n)), float64(end)))
		view := js.Global().Get("Uint8Array").New(buffer)
		copied := js.CopyBytesToGo(dst[n:], view)
		if copied != int(end-offset-int64(n)) {
			return n, io.ErrUnexpectedEOF
		}
		n += copied
	}
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}
func (r *fileReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.file = js.Undefined()
	r.reader = js.Undefined()
	return nil
}
