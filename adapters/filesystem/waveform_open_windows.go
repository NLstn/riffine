//go:build windows

package filesystem

import (
	"os"
	"syscall"
)

// A reader retains its original file across path replacement or relocation.
// Windows requires delete sharing; access remains read-only. Caches use the same
// resource open so replacing a cache leaves its old complete reader usable.
func openResourceFile(path string) (*os.File, error) {
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

func openPeakCache(path string) (*os.File, error) { return openResourceFile(path) }
