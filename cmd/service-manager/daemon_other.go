//go:build !windows

package main

// daemonize is a no-op on platforms without the console-detach problem: the
// service manager stays in the foreground because it is normally launched by
// a system service manager (e.g. systemd, detected via INVOCATION_ID). It
// always returns false, so the process runs as normal.
func daemonize() bool { return false }
