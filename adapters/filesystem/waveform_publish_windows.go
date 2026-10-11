//go:build windows

package filesystem

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// ReplaceFileW permits an immutable cache reader to retain the old complete
// file. MoveFileEx (os.Rename) is used only when the target does not yet exist.
// Brief sharing conflicts from scanners or concurrent publication are retried
// within 250ms; cancellation and permanent failures remain explicit. Never
// delete the destination to work around a sharing failure.
func publishPeakCache(ctx context.Context, staged, path string) error {
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := replaceDocument(staged, path)
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			err = os.Rename(staged, path)
		}
		if err == nil {
			return nil
		}
		if (!errors.Is(err, syscall.ERROR_ACCESS_DENIED) && !errors.Is(err, syscall.Errno(32)) && !errors.Is(err, syscall.Errno(33))) || time.Now().After(deadline) {
			return err
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
