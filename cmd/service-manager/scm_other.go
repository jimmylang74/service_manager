//go:build !windows

package main

import (
	"os"
	"time"
)

func runWithSCM(_ string, fn func(stopCh chan struct{})) {
	stopCh := make(chan struct{})
	fn(stopCh)
}

func isSCM() bool {
	return os.Getenv("INVOCATION_ID") != ""
}

func scmSleep(d time.Duration) {
	time.Sleep(d)
}
