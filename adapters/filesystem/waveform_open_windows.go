//go:build windows

package filesystem

import (
	"os"
	"syscall"
)

// Generated caches may be replaced while a reader holds the old complete file.
// Windows requires explicit delete sharing for this immutable-reader behavior.
func openPeakCache(path string) (*os.File, error) {
	name, err := windowsDocumentPath(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
