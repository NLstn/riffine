//go:build !windows

package filesystem

import "os"

func openResourceFile(path string) (*os.File, error) { return os.Open(path) }

func openPeakCache(path string) (*os.File, error) { return openResourceFile(path) }
