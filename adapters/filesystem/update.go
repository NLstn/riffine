package filesystem

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
)

var _ resource.UpdatingStore = Host{}

func (h Host) Update(ctx context.Context, path, revision string, data []byte) (resource.UpdateResult, error) {
	return h.update(ctx, path, revision, data, nil)
}

// beforeCommit is a private fault-injection boundary used by filesystem tests.
func (h Host) update(ctx context.Context, path, revision string, data []byte, beforeCommit func() error) (resource.UpdateResult, error) {
	var result resource.UpdateResult
	if err := ctx.Err(); err != nil {
		return result, err
	}
	digest, err := hex.DecodeString(revision)
	if err != nil || len(digest) != 32 {
		return result, errors.New("invalid expected document revision")
	}
	if _, err = project.Decode(bytes.NewReader(data)); err != nil {
		return result, err
	}
	// Reject final-component symlinks, directories and non-project files. Resolve
	// parent aliases so cooperating writers use the same cross-process lock name.
	st, err := os.Lstat(path)
	if err != nil {
		return result, err
	}
	if !st.Mode().IsRegular() {
		return result, errors.New("document update requires a regular non-symlink file")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return result, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return result, err
	}
	lock := path + ".riffine-lock"
	if err = os.Mkdir(lock, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return result, resource.ErrDocumentBusy
		}
		return result, err
	}
	defer os.Remove(lock)
	old, err := h.Read(ctx, path, project.MaxDocumentBytes)
	if err != nil {
		return result, err
	}
	if resource.Revision(old) != revision {
		return result, resource.ErrRevisionConflict
	}
	if _, err = project.Decode(bytes.NewReader(old)); err != nil {
		return result, errors.New("refusing to replace a non-project document")
	}
	recovery := path + ".recovery-" + revision + ".json"
	if err = h.Create(ctx, recovery, old); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return result, err
		}
		backup, e := h.Read(ctx, recovery, project.MaxDocumentBytes)
		if e != nil || !bytes.Equal(backup, old) {
			return result, errors.New("existing recovery copy does not match previous revision")
		}
	}
	result.Recovery = recovery
	staged, err := os.CreateTemp(filepath.Dir(path), ".riffine-update-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(staged.Name())
	defer staged.Close()
	if _, err = staged.Write(data); err != nil {
		return result, err
	}
	if err = staged.Sync(); err != nil {
		return result, err
	}
	if err = staged.Close(); err != nil {
		return result, err
	}
	if err = syncDocumentDirectory(filepath.Dir(path)); err != nil {
		return result, err
	}
	if beforeCommit != nil {
		if err = beforeCommit(); err != nil {
			return result, err
		}
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	// No truncate or delete-before-rename fallback. The previous synced recovery
	// copy survives even when the host replacement call reports an ambiguous error.
	if err = replaceDocument(staged.Name(), path); err != nil {
		return result, err
	}
	result.Revision = resource.Revision(data)
	return result, nil
}
