//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	advapi32                         = syscall.NewLazyDLL("advapi32.dll")
	procSetServiceStatus             = advapi32.NewProc("SetServiceStatus")
	procRegisterServiceCtrlHandlerExW = advapi32.NewProc("RegisterServiceCtrlHandlerExW")
)

const (
	_SERVICE_WIN32_OWN_PROCESS = 0x00000010
	_SERVICE_RUNNING           = 0x00000004
	_SERVICE_START_PENDING     = 0x00000002
	_SERVICE_STOPPED            = 0x00000001
	_SERVICE_ACCEPT_STOP       = 0x00000001
	_SERVICE_ACCEPT_SHUTDOWN   = 0x00000004
)

type serviceStatus struct {
	dwServiceType             uint32
	dwCurrentState            uint32
	dwControlsAccepted        uint32
	dwWin32ExitCode           uint32
	dwServiceSpecificExitCode uint32
	dwCheckPoint              uint32
	dwWaitHint                uint32
}

func runWithSCM(name string, fn func(stopCh chan struct{})) {
	h := registerHandler(name)
	if h == 0 {
		fmt.Fprintln(os.Stderr, "SCM handler unavailable, running as console app")
		stopCh := make(chan struct{})
		fn(stopCh)
		return
	}

	reportStatus(h, _SERVICE_START_PENDING, 3000)

	stopCh := make(chan struct{})
	done := make(chan struct{})

	go func() {
		fn(stopCh)
		close(done)
	}()

	reportStatus(h, _SERVICE_RUNNING, 0)

	select {
	case <-stopCh:
	case <-done:
	}
	reportStatus(h, _SERVICE_STOPPED, 0)
}

func registerHandler(name string) uintptr {
	n, _ := syscall.UTF16PtrFromString(name)
	h, _, _ := procRegisterServiceCtrlHandlerExW.Call(
		uintptr(unsafe.Pointer(n)),
		0,
		0,
	)
	return h
}

func reportStatus(h uintptr, state uint32, waitHint uint32) {
	s := serviceStatus{
		dwServiceType:   _SERVICE_WIN32_OWN_PROCESS,
		dwCurrentState:  state,
		dwWin32ExitCode: 0,
		dwWaitHint:      waitHint,
	}
	if state == _SERVICE_RUNNING || state == _SERVICE_STOPPED {
		s.dwControlsAccepted = _SERVICE_ACCEPT_STOP | _SERVICE_ACCEPT_SHUTDOWN
	}
	procSetServiceStatus.Call(h, uintptr(unsafe.Pointer(&s)))
}

func isSCM() bool {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "service", "stop", "pause", "continue", "shutdown":
			return true
		}
	}
	return false
}

func scmSleep(d time.Duration) {
	time.Sleep(d)
}
