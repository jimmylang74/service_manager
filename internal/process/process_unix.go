//go:build !windows

package process

import "os/exec"

func setNoWindow(cmd *exec.Cmd) {}

