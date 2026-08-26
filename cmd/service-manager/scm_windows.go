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
	advapi32                = syscall.NewLazyDLL("advapi32.dll")
	procSetServiceStatus    = advapi32.NewProc("SetServiceStatus")
	procRegisterSCCtrlHandler = advapi32.NewProc("RegisterServiceCtrlHandlerW")
	procStartSCDispatcher   = advapi32.NewProc("StartServiceCtrlDispatcherW")
)

const (
	_SERVICE_WIN32_OWN_PROCESS = 0x00000010
	_SERVICE_RUNNING           = 0x00000004
	_SERVICE_START_PENDING     = 0x00000002
	_SERVICE_STOPPED           = 0x00000001
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

type serviceTableEntry struct {
	lpServiceName *uint16
	lpServiceProc uintptr
}

type serviceMainFunc func(argc uint32, argv **uint16)

var (
	scmName     string
	scmStopCh   chan struct{}
	scmMainFunc func(stopCh chan struct{})
)

func runWithSCM(name string, fn func(stopCh chan struct{})) {
	scmName = name
	scmMainFunc = fn
	scmStopCh = make(chan struct{})

	namePtr, _ := syscall.UTF16PtrFromString(name)
	entry := serviceTableEntry{
		lpServiceName: namePtr,
		lpServiceProc: syscall.NewCallback(scmServiceMain),
	}
	table := [2]serviceTableEntry{entry, {}}

	ret, _, _ := procStartSCDispatcher.Call(uintptr(unsafe.Pointer(&table[0])))
	if ret == 0 {
		fmt.Fprintln(os.Stderr, "StartServiceCtrlDispatcher failed, not running under SCM")
		fn(make(chan struct{}))
		return
	}

	<-scmStopCh
}

func scmServiceMain(argc uint32, argv **uint16) {
	h := registerHandler(scmName)
	if h == 0 {
		scmStopCh <- struct{}{}
		return
	}

	reportStatus(h, _SERVICE_START_PENDING, 3000)

	done := make(chan struct{})
	go func() {
		scmMainFunc(scmStopCh)
		close(done)
	}()

	reportStatus(h, _SERVICE_RUNNING, 0)

	select {
	case <-scmStopCh:
	case <-done:
	}
	reportStatus(h, _SERVICE_STOPPED, 0)
}

func registerHandler(name string) uintptr {
	n, _ := syscall.UTF16PtrFromString(name)
	h, _, _ := procRegisterSCCtrlHandler.Call(
		uintptr(unsafe.Pointer(n)),
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
	return false
}

func scmSleep(d time.Duration) {
	time.Sleep(d)
}
