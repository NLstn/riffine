//go:build !windows

package native

import (
	"os"
	"os/exec"
)

func inheritSource(cmd *exec.Cmd, file *os.File) (uint64, func(), error) {
	// os/exec duplicates the descriptor into the child; it does not close the
	// borrowed parent File. Native offset reads do not share a mutable file cursor.
	cmd.ExtraFiles = []*os.File{file}
	return 3, func() {}, nil
}
