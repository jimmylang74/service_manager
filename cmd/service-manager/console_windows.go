//go:build windows

package main

import "golang.org/x/sys/windows"

// procFreeConsole detaches the calling process from its attached console.
// FreeConsole is not wrapped by golang.org/x/sys/windows, so it is invoked
// through x/sys's system DLL loader, which searches System32 only.
var procFreeConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")

// detachConsole detaches from the console so that normal runs do not keep a
// console window open. It must only be called after flag parsing so that
// --help / -h output is still visible in the console before detaching.
func detachConsole() {
	procFreeConsole.Call()
}
