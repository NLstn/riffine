//go:build !windows

package filesystem

import (
	"context"
	"os"
)

func publishPeakCache(ctx context.Context, staged, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(staged, path)
}
