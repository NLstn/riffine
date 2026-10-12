package native

import (
	"os"
	"os/exec"
	"syscall"
)

func inheritSource(cmd *exec.Cmd, file *os.File) (uint64, func(), error) {
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, nil, err
	}
	var duplicate syscall.Handle
	if err = syscall.DuplicateHandle(process, syscall.Handle(file.Fd()), process, &duplicate, 0, true, syscall.DUPLICATE_SAME_ACCESS); err != nil {
		return 0, nil, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{duplicate}}
	return uint64(duplicate), func() { syscall.CloseHandle(duplicate) }, nil
}
