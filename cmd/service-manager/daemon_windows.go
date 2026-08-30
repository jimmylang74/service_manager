//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// daemonEnvVar marks a process as the detached background copy of the service
// manager. Its absence means the process was launched interactively from a
// console and should re-spawn a detached copy of itself before exiting.
var daemonEnvVar = "SERVICE_MANAGER_DAEMON"

// daemonize re-executes the current executable as a detached background
// process that owns no console (CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS), so
// the original console-launched process can exit immediately and return
// control to the command prompt.
//
// It returns true when a detached copy was spawned — the caller (the launcher)
// should then exit(0). It returns false when SERVICE_MANAGER_DAEMON is already
// set (this process IS the detached daemon and must run as normal) or when no
// copy could be spawned (the process should run in the foreground instead).
func daemonize() bool {
	if os.Getenv(daemonEnvVar) != "" {
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		return false
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
	// Inherit the full environment plus the marker so the copy takes the
	// daemon branch and never re-spawns.
	cmd.Env = append(os.Environ(), daemonEnvVar+"=1")
	// The detached copy must not hold references to the console's standard
	// handles, or cmd.exe may wait on it before showing the prompt again.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return false
	}
	// Release the handle so the launcher does not Wait() on the long-lived
	// daemon; the launcher exits right after.
	_ = cmd.Process.Release()
	return true
}
