package native

import (
	"os"
	"os/exec"
	"testing"
)

func TestHiddenHelperRetainsBorrowedHandle(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "source")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cmd := exec.Command("unused-helper")
	configureHelper(cmd)
	handle, release, err := inheritSource(cmd, file)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 ||
		len(cmd.SysProcAttr.AdditionalInheritedHandles) != 1 || uint64(cmd.SysProcAttr.AdditionalInheritedHandles[0]) != handle {
		t.Fatal("borrowing a source discarded hidden-window flags or inherited handle")
	}
}
