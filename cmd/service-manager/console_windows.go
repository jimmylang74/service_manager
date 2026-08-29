//go:build windows

package main

import "syscall"

// procFreeConsole detaches the calling process from its attached console.
var procFreeConsole = syscall.NewLazyDLL("kernel32.dll").NewProc("FreeConsole")

// detachConsole detaches from the console so that normal runs do not keep a
// console window open. It must only be called after flag parsing so that
// --help / -h output is still visible in the console before detaching.
func detachConsole() {
	procFreeConsole.Call()
}