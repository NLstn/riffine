//go:build windows

package filesystem

import (
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var replaceFileW = syscall.NewLazyDLL("kernel32.dll").NewProc("ReplaceFileW")

func windowsDocumentPath(path string) (*uint16, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, `\\?\`) {
		if strings.HasPrefix(path, `\\`) {
			path = `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
		} else {
			path = `\\?\` + path
		}
	}
	return syscall.UTF16PtrFromString(path)
}

// ReplaceFileW preserves the existing file's attributes/ACLs. The separately
// synced recovery file is kept on every path; no destructive retry is attempted.
func replaceDocument(staged, path string) error {
	target, err := windowsDocumentPath(path)
	if err != nil {
		return err
	}
	source, err := windowsDocumentPath(staged)
	if err != nil {
		return err
	}
	ok, _, callErr := replaceFileW.Call(uintptr(unsafe.Pointer(target)), uintptr(unsafe.Pointer(source)), 0, 0, 0, 0)
	runtime.KeepAlive(target)
	runtime.KeepAlive(source)
	if ok == 0 {
		if callErr == syscall.Errno(0) {
			return syscall.EINVAL
		}
		return callErr
	}
	return nil
}

// Windows has no portable directory fsync. Synced file contents and the native
// replace operation are used; power-loss durability is a separate hardware gate.
func syncDocumentDirectory(string) error { return nil }
