//go:build windows

package process

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func setNoWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}
