//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

// Windows API constants. Values verified against golang.org/x/sys/windows
// (Go 1.22 toolchain sources); kept local to preserve the zero-dependency
// build.
const (
	createNoWindow         = 0x08000000
	createSuspended        = 0x00000004
	createBreakawayFromJob = 0x01000000

	jobObjectLimitKillOnJobClose = 0x00002000
	jobObjectExtendedLimitInfo   = 9

	processSetQuota         = 0x0100
	processTerminate        = 0x0001
	processSuspendResume    = 0x0800
	processQueryInformation = 0x0400
)

// jobObjectBasicLimitInformation mirrors JOBOBJECT_BASIC_LIMIT_INFORMATION.
// The layout is 64 bytes on amd64; the manager builds for amd64 only.
type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

// ioCounters mirrors IO_COUNTERS (48 bytes).
type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// jobObjectExtendedLimitInformation mirrors JOBOBJECT_EXTENDED_LIMIT_INFORMATION
// (144 bytes on amd64, asserted against the Windows SDK layout). A wrong layout
// would silently corrupt the job configuration, hence the exact field-for-field
// copy.
type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	ntdll                = syscall.NewLazyDLL("ntdll.dll")
	procCreateJobObjectW = kernel32.NewProc("CreateJobObjectW")
	procSetInfoJobObject = kernel32.NewProc("SetInformationJobObject")
	procAssignProcToJob  = kernel32.NewProc("AssignProcessToJobObject")
	procOpenProcess      = kernel32.NewProc("OpenProcess")
	procNtResumeProcess  = ntdll.NewProc("NtResumeProcess")
)

var (
	jobOnce   sync.Once
	jobHandle syscall.Handle
	jobErr    error
)

// managerJob returns the process-wide job object that confines every managed
// service process to the manager's lifetime. The job is configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: when the manager process exits for any
// reason (normal exit, crash, or forced termination), Windows closes all of
// its handles; the last job handle being closed forces the OS to terminate
// every process in the job, including grandchildren spawned by the services.
//
// The handle is deliberately kept open for the whole lifetime of the manager
// and is never closed during normal operation; the OS reclaims it at process
// exit. It is created without security attributes, which makes it
// non-inheritable: no child process can ever hold a duplicate that would keep
// the job alive after the manager dies.
func managerJob() (syscall.Handle, error) {
	jobOnce.Do(func() {
		h, _, e1 := procCreateJobObjectW.Call(0, 0) // nil attributes -> non-inheritable, unnamed
		if h == 0 {
			jobErr = e1
			return
		}
		info := jobObjectExtendedLimitInformation{
			BasicLimitInformation: jobObjectBasicLimitInformation{
				LimitFlags: jobObjectLimitKillOnJobClose,
			},
		}
		if ret, _, e1 := procSetInfoJobObject.Call(
			h,
			jobObjectExtendedLimitInfo,
			uintptr(unsafe.Pointer(&info)),
			uintptr(unsafe.Sizeof(info)),
		); ret == 0 {
			_ = syscall.CloseHandle(syscall.Handle(h))
			jobErr = e1
			return
		}
		jobHandle = syscall.Handle(h)
	})
	return jobHandle, jobErr
}

// setNoWindow keeps a console window from flashing when the service starts.
func setNoWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}

// startWithJob starts a managed service and confines it to the manager's job
// object, so the service (and everything it spawns) dies together with the
// manager.
//
// The process is created suspended (CREATE_SUSPENDED), assigned to the job,
// and only then resumed. This closes the race window in which a child could
// exit on its own or spawn grandchildren that escape the job before the
// assignment happens.
//
// When running as a Windows SCM service, services.exe may already confine the
// manager to its own job; children then inherit that membership and the
// assignment can fail with ERROR_ACCESS_DENIED. In that case the suspended
// child is terminated and the service is recreated with
// CREATE_BREAKAWAY_FROM_JOB. If the assignment still fails, the service is
// resumed and runs without job confinement: a managed service must never hang
// or fail to start just because the job could not be set up.
func startWithJob(newCmd func() (*exec.Cmd, error), logf func(format string, args ...interface{})) (*exec.Cmd, error) {
	job, err := managerJob()
	if err != nil {
		logf("manager job unavailable (%v), starting without job confinement", err)
		return plainStart(newCmd)
	}

	cmd, h, err := spawnSuspended(newCmd, 0)
	if err != nil {
		return nil, err
	}
	if err := assignToJob(job, h); err != nil {
		logf("job assignment failed for pid %d (%v), retrying with breakaway", cmd.Process.Pid, err)
		killSuspended(cmd, h)
		cmd, h, err = spawnSuspended(newCmd, createBreakawayFromJob)
		if err != nil {
			return nil, err
		}
		if err := assignToJob(job, h); err != nil {
			resumeProcess(h, logf) // best effort: run unconfined instead of hanging
			_ = syscall.CloseHandle(h)
			logf("job assignment failed for pid %d (%v); service runs without job confinement", cmd.Process.Pid, err)
			return cmd, nil
		}
	}
	resumeProcess(h, logf)
	_ = syscall.CloseHandle(h)
	return cmd, nil
}

// plainStart starts a command exactly as os/exec would on its own.
func plainStart(newCmd func() (*exec.Cmd, error)) (*exec.Cmd, error) {
	cmd, err := newCmd()
	if err != nil {
		return nil, err
	}
	setNoWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// spawnSuspended creates the command suspended with the given extra creation
// flags and opens an additional process handle with the rights needed for job
// assignment and resumption.
func spawnSuspended(newCmd func() (*exec.Cmd, error), extraFlags uint32) (*exec.Cmd, syscall.Handle, error) {
	cmd, err := newCmd()
	if err != nil {
		return nil, 0, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow | createSuspended | extraFlags,
	}
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	h, err := openProcessHandle(uint32(cmd.Process.Pid))
	if err != nil {
		// Cannot resume without a handle; kill the suspended child rather
		// than leave it hanging forever.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, 0, fmt.Errorf("open handle for pid %d: %w", cmd.Process.Pid, err)
	}
	return cmd, h, nil
}

// openProcessHandle opens a process handle with the rights needed to assign
// the process to a job (PROCESS_SET_QUOTA|PROCESS_TERMINATE) and to resume it
// (PROCESS_SUSPEND_RESUME).
func openProcessHandle(pid uint32) (syscall.Handle, error) {
	h, _, e1 := procOpenProcess.Call(
		processSetQuota|processTerminate|processSuspendResume|processQueryInformation,
		0,
		uintptr(pid),
	)
	if h == 0 {
		return 0, e1
	}
	return syscall.Handle(h), nil
}

// assignToJob moves a process into the given job object.
func assignToJob(job, process syscall.Handle) error {
	ret, _, e1 := procAssignProcToJob.Call(uintptr(job), uintptr(process))
	if ret == 0 {
		return e1
	}
	return nil
}

// resumeProcess resumes a process created with CREATE_SUSPENDED by resuming
// all of its threads (NtResumeProcess).
func resumeProcess(h syscall.Handle, logf func(format string, args ...interface{})) {
	if ret, _, _ := procNtResumeProcess.Call(uintptr(h)); ret != 0 {
		logf("NtResumeProcess failed (status 0x%x), process may stay suspended", ret)
	}
}

// killSuspended terminates and reaps a suspended process whose job assignment
// failed, so the start can be retried cleanly.
func killSuspended(cmd *exec.Cmd, h syscall.Handle) {
	_ = syscall.TerminateProcess(h, 1)
	_ = syscall.CloseHandle(h)
	_ = cmd.Wait()
}
