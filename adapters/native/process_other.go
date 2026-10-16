//go:build !windows

package native

import "os/exec"

func configureHelper(*exec.Cmd) {}
