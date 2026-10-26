//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

func prepareSlicerProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000 | 0x00004000, HideWindow: true}
}
