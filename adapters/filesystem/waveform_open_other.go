//go:build !windows

package filesystem

import "os"

func openPeakCache(path string) (*os.File, error) { return os.Open(path) }
