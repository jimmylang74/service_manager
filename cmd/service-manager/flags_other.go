//go:build !windows

package main

// scmOpt stays nil on non-Windows platforms, so the -scm flag does not exist
// there: service mode is detected automatically from the environment
// (systemd INVOCATION_ID).
var scmOpt *bool

func registerSCMFlag() {}