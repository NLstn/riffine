// Package filesystem owns local paths, file access and exclusive publication.
package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

type Host struct{}

var _ resource.Resources = Host{}
var _ resource.Store = Host{}

type reader struct {
	*os.File
	size int64
}

func (r reader) Size() int64  { return r.size }
func (r reader) Name() string { return filepath.Base(r.File.Name()) }
func (Host) Open(ctx context.Context, handle string) (resource.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := openResourceFile(handle)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("resource is not a regular file")
	}
	return reader{f, st.Size()}, nil
}
func (Host) Resolve(ctx context.Context, document, reference string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if filepath.IsAbs(filepath.FromSlash(reference)) {
		return filepath.FromSlash(reference), nil
	}
	return filepath.Join(filepath.Dir(document), filepath.FromSlash(reference)), nil
}
func (Host) Reference(ctx context.Context, document, source string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	dir, err := filepath.Abs(filepath.Dir(document))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil {
		rel = abs
	}
	return filepath.ToSlash(rel), nil
}
func (h Host) Read(ctx context.Context, document string, maxBytes int) ([]byte, error) {
	if maxBytes < 1 {
		return nil, errors.New("invalid document limit")
	}
	r, err := h.Open(ctx, document)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if r.Size() > int64(maxBytes) {
		return nil, errors.New("document exceeds size limit")
	}
	b, err := io.ReadAll(io.NewSectionReader(r, 0, int64(maxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBytes {
		return nil, errors.New("document exceeds size limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b, nil
}

// Create stages and syncs bytes before publishing with an exclusive hard link.
// Source audio and prior documents are never truncated. Unsupported filesystems
// fail explicitly; there is no non-atomic overwrite fallback.
func (Host) Create(ctx context.Context, path string, b []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".riffine-save-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}

func Inspect(ctx context.Context, path string) (audio.Info, error) {
	r, err := (Host{}).Open(ctx, path)
	if err != nil {
		return audio.Info{}, err
	}
	defer r.Close()
	return audio.Inspect(ctx, r)
}

// WriteTone keeps generated fixtures and their no-overwrite policy in the host.
func WriteTone(path string, seconds, hz float64, sampleRate, channels int) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".riffine-tone-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = audio.WriteTone(f, seconds, hz, sampleRate, channels); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(f.Name(), path)
}

// LocalPath exposes a verified host-owned regular file only to filesystem-aware
// adapters. Core code must use the borrowed resource instead of interpreting it.
func LocalPath(r resource.Reader) (string, bool) {
	local, ok := r.(reader)
	if !ok {
		return "", false
	}
	return local.File.Name(), true
}

// BorrowedFile lends a regular resource's original read-only File to host process
// adapters. The caller must retain it until inherited helper teardown finishes;
// adapters must not close it or change its file position. Core uses ReaderAt.
func BorrowedFile(r resource.Reader) (*os.File, bool) {
	local, ok := r.(reader)
	if !ok {
		return nil, false
	}
	return local.File, true
}
