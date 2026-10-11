package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/NLstn/riffine/core/waveform"
)

// PeakCache owns only generated cache files in a caller-selected directory.
// Opaque keys are hashed, so they cannot escape that directory. Audio/project
// files are never interpreted as cache paths. Concurrent complete writes may
// replace each other; identical keys must represent identical algorithms/source.
type PeakCache struct{ Directory string }

func (c PeakCache) path(key string) (string, error) {
	if c.Directory == "" || key == "" || len(key) > 512 {
		return "", errors.New("invalid waveform cache directory or key")
	}
	hash := sha256.Sum256([]byte(key))
	return filepath.Join(c.Directory, hex.EncodeToString(hash[:])+".peaks"), nil
}
func (c PeakCache) Get(ctx context.Context, key string, maxBytes int) ([]byte, error) {
	path, err := c.path(key)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes < 1 || maxBytes > waveform.MaxCacheBytes {
		return nil, errors.New("invalid waveform cache read limit")
	}
	file, err := openPeakCache(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > int64(maxBytes) {
		return nil, errors.New("invalid or oversized waveform cache")
	}
	data, err := io.ReadAll(io.LimitReader(peakCacheReader{ctx: ctx, source: file}, int64(maxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, errors.New("waveform cache changed beyond its read limit")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
func (c PeakCache) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) > waveform.MaxCacheBytes {
		return errors.New("waveform cache exceeds size limit")
	}
	path, err := c.path(key)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(c.Directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(c.Directory, ".riffine-peaks-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Cache publication uses the platform rename replacement operation. Failure
	// preserves the prior complete cache; caller can still use the new pyramid.
	return publishPeakCache(ctx, file.Name(), path)
}

var _ waveform.Cache = PeakCache{}

type peakCacheReader struct {
	ctx    context.Context
	source io.Reader
}

func (r peakCacheReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(data) > 64<<10 {
		data = data[:64<<10]
	}
	return r.source.Read(data)
}
