//go:build !windows

package main

// detachConsole is a no-op on platforms without a separate console concept.
func detachConsole() {}