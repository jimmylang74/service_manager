//go:build !windows

package process

import "os/exec"

func setNoWindow(cmd *exec.Cmd) {}

// startWithJob starts the managed process. Non-Windows platforms have no job
// object confinement; the process is started exactly as before.
func startWithJob(newCmd func() (*exec.Cmd, error), logf func(format string, args ...interface{})) (*exec.Cmd, error) {
	cmd, err := newCmd()
	if err != nil {
		return nil, err
	}
	setNoWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
