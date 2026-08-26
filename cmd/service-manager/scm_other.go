//go:build !windows

package main

import "time"

func runWithSCM(_ string, fn func(stopCh chan struct{})) {
	stopCh := make(chan struct{})
	fn(stopCh)
}

func isSCM() bool {
	return false
}

func scmSleep(d time.Duration) {
	time.Sleep(d)
}
