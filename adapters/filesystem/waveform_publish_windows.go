//go:build windows

package filesystem

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var setPeakFileInformation = syscall.NewLazyDLL("kernel32.dll").NewProc("SetFileInformationByHandle")

type peakRenameInfo struct {
	Flags          uint32
	RootDirectory  syscall.Handle
	FileNameLength uint32
	FileName       [32768]uint16
}

// FileRenameInfoEx with POSIX replacement keeps the name continuously available:
// existing readers retain the old complete file, and new opens see the new file.
// ReplaceFileW can expose a missing-name interval; MoveFileEx can reject open
// readers. An unsupported filesystem returns a cache diagnostic, never a
// destructive remove/rename fallback. Source/project persistence is separate.
func publishPeakCache(ctx context.Context, staged, path string) error {
	source, err := windowsDocumentPath(staged)
	if err != nil {
		return err
	}
	const deleteAccess = 0x00010000
	handle, err := syscall.CreateFile(source, deleteAccess, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	runtime.KeepAlive(source)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	name, err := syscall.UTF16FromString(absolute)
	if err != nil {
		return err
	}
	if len(name) > 32768 {
		return syscall.ENAMETOOLONG
	}
	info := peakRenameInfo{Flags: 3, FileNameLength: uint32((len(name) - 1) * 2)}
	copy(info.FileName[:], name)
	const fileRenameInfoEx = 22
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		ok, _, callErr := setPeakFileInformation.Call(uintptr(handle), fileRenameInfoEx, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
		runtime.KeepAlive(&info)
		if ok != 0 {
			return nil
		}
		if callErr == syscall.Errno(0) {
			callErr = syscall.EINVAL
		}
		if (!errors.Is(callErr, syscall.ERROR_ACCESS_DENIED) && !errors.Is(callErr, syscall.Errno(32)) && !errors.Is(callErr, syscall.Errno(33))) || time.Now().After(deadline) {
			return callErr
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
