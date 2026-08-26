//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	advapi32                    = syscall.NewLazyDLL("advapi32.dll")
	procSetServiceStatus        = advapi32.NewProc("SetServiceStatus")
	procRegisterSCCtrlHandler   = advapi32.NewProc("RegisterServiceCtrlHandlerExW")
	procStartSCDispatcher       = advapi32.NewProc("StartServiceCtrlDispatcherW")
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	ntdll                       = syscall.NewLazyDLL("ntdll.dll")
	_PROCESS_QUERY_LIMITED_INFORMATION uint32 = 0x1000
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

var (
	scmName     string
	scmStopCh   chan struct{}
	scmMainFunc func(stopCh chan struct{})
	scmHandler  uintptr
	scmLogFile  *os.File
)

func scmLog(format string, args ...interface{}) {
	msg := fmt.Sprintf("[SCM] "+format, args...)
	if scmLogFile != nil {
		fmt.Fprintln(scmLogFile, msg)
		scmLogFile.Sync()
	}
	fmt.Println(msg)
}

func initSCMLog() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	logPath := filepath.Join(filepath.Dir(exe), "scm_debug.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open scm_debug.log: %v\n", err)
		return
	}
	scmLogFile = f
	fmt.Fprintln(scmLogFile, "=== SCM debug log started ===")
}

func runWithSCM(name string, fn func(stopCh chan struct{})) {
	initSCMLog()
	scmLog("runWithSCM called, name: %s", name)
	scmName = name
	scmMainFunc = fn
	scmStopCh = make(chan struct{})

	namePtr, _ := syscall.UTF16PtrFromString(name)
	entry := serviceTableEntry{
		lpServiceName: namePtr,
		lpServiceProc: syscall.NewCallback(scmServiceMain),
	}
	table := [2]serviceTableEntry{entry, {}}

	scmLog("calling StartServiceCtrlDispatcher...")
	ret, _, errNo := procStartSCDispatcher.Call(uintptr(unsafe.Pointer(&table[0])))
	if ret == 0 {
		scmLog("StartServiceCtrlDispatcher FAILED (error %d), not under SCM", errNo)
		scmLog("running as console app")
		fn(make(chan struct{}))
		return
	}

	scmLog("StartServiceCtrlDispatcher succeeded, waiting for stop...")
	<-scmStopCh
	scmLog("stop received, exiting")
}

func scmServiceMain(argc uint32, argv **uint16) {
	scmLog("scmServiceMain called, argc: %d", argc)

	scmHandler, _, _ = procRegisterSCCtrlHandler.Call(
		uintptr(unsafe.Pointer(&scmName)),
		0,
	)
	if scmHandler == 0 {
		scmLog("RegisterServiceCtrlHandler FAILED")
		scmStopCh <- struct{}{}
		return
	}
	scmLog("RegisterServiceCtrlHandler OK, handler: %d", scmHandler)

	s := serviceStatus{
		dwServiceType:   _SERVICE_WIN32_OWN_PROCESS,
		dwCurrentState:  _SERVICE_START_PENDING,
		dwWaitHint:      30000,
		dwCheckPoint:    1,
	}
	procSetServiceStatus.Call(scmHandler, uintptr(unsafe.Pointer(&s)))
	scmLog("reported SERVICE_START_PENDING")

	done := make(chan struct{})
	go func() {
		scmLog("starting daemon...")
		scmMainFunc(scmStopCh)
		scmLog("daemon exited")
		close(done)
	}()

	s.dwCurrentState = _SERVICE_RUNNING
	s.dwControlsAccepted = _SERVICE_ACCEPT_STOP | _SERVICE_ACCEPT_SHUTDOWN
	s.dwWaitHint = 0
	s.dwCheckPoint = 0
	procSetServiceStatus.Call(scmHandler, uintptr(unsafe.Pointer(&s)))
	scmLog("reported SERVICE_RUNNING")

	select {
	case <-scmStopCh:
		scmLog("stop signal received")
	case <-done:
		scmLog("daemon finished")
	}

	s.dwCurrentState = _SERVICE_STOPPED
	s.dwControlsAccepted = 0
	procSetServiceStatus.Call(scmHandler, uintptr(unsafe.Pointer(&s)))
	scmLog("reported SERVICE_STOPPED")
}

func isSCM() bool {
	ppid := getParentPID()
	if ppid == 0 {
		scmLog("isSCM: cannot get parent PID")
		return false
	}
	name := getProcessNameByPID(ppid)
	scmLog("isSCM: parent PID=%d, name=%s", ppid, name)
	return strings.EqualFold(name, "services.exe")
}

type processBasicInformation struct {
	Reserved1                    uintptr
	PebBaseAddress               uintptr
	Reserved2_0                  uintptr
	Reserved2_1                  uintptr
	UniqueProcessId              uintptr
	InheritedFromUniqueProcessId uintptr
}

func getParentPID() uint32 {
	var pbi processBasicInformation
	status, _, _ := ntdll.NewProc("NtQueryInformationProcess").Call(
		^uintptr(0),
		0,
		uintptr(unsafe.Pointer(&pbi)),
		unsafe.Sizeof(pbi),
		0,
	)
	if status != 0 {
		scmLog("NtQueryInformationProcess failed: %d", status)
		return 0
	}
	return uint32(pbi.InheritedFromUniqueProcessId)
}

func getProcessNameByPID(pid uint32) string {
	handle, _ := syscall.OpenProcess(_PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if handle == 0 {
		scmLog("OpenProcess failed for PID %d", pid)
		return ""
	}
	defer syscall.CloseHandle(handle)

	var buf [260]uint16
	size := uint32(len(buf))
	ret, _, _ := procQueryFullProcessImageNameW.Call(
		uintptr(handle),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		scmLog("QueryFullProcessImageName failed")
		return ""
	}
	path := syscall.UTF16ToString(buf[:size])
	parts := strings.Split(path, "\\")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func scmSleep(d time.Duration) {
	time.Sleep(d)
}
