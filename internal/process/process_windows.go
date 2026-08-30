//go:build windows

package process

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// procNtResumeProcess resumes every thread of a suspended process.
// NtResumeProcess is not wrapped by golang.org/x/sys/windows, so it is invoked
// through x/sys's system DLL loader, which searches System32 only.
var procNtResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

var (
	jobOnce   sync.Once
	jobHandle windows.Handle
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
func managerJob() (windows.Handle, error) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil) // nil attributes -> non-inheritable, unnamed
		if err != nil {
			jobErr = err
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(
			h,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			_ = windows.CloseHandle(h)
			jobErr = err
			return
		}
		jobHandle = h
	})
	return jobHandle, jobErr
}

// setNoWindow keeps a console window from flashing when the service starts.
func setNoWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW,
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
		cmd, h, err = spawnSuspended(newCmd, windows.CREATE_BREAKAWAY_FROM_JOB)
		if err != nil {
			return nil, err
		}
		if err := assignToJob(job, h); err != nil {
			resumeProcess(h, logf) // best effort: run unconfined instead of hanging
			_ = windows.CloseHandle(h)
			logf("job assignment failed for pid %d (%v); service runs without job confinement", cmd.Process.Pid, err)
			return cmd, nil
		}
	}
	resumeProcess(h, logf)
	_ = windows.CloseHandle(h)
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
func spawnSuspended(newCmd func() (*exec.Cmd, error), extraFlags uint32) (*exec.Cmd, windows.Handle, error) {
	cmd, err := newCmd()
	if err != nil {
		return nil, 0, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED | extraFlags,
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
func openProcessHandle(pid uint32) (windows.Handle, error) {
	return windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME|windows.PROCESS_QUERY_INFORMATION,
		false,
		pid,
	)
}

// assignToJob moves a process into the given job object.
func assignToJob(job, process windows.Handle) error {
	return windows.AssignProcessToJobObject(job, process)
}

// resumeProcess resumes a process created with CREATE_SUSPENDED by resuming
// all of its threads (NtResumeProcess).
func resumeProcess(h windows.Handle, logf func(format string, args ...interface{})) {
	if ret, _, _ := procNtResumeProcess.Call(uintptr(h)); ret != 0 {
		logf("NtResumeProcess failed (status 0x%x), process may stay suspended", ret)
	}
}

// killSuspended terminates and reaps a suspended process whose job assignment
// failed, so the start can be retried cleanly.
func killSuspended(cmd *exec.Cmd, h windows.Handle) {
	_ = windows.TerminateProcess(h, 1)
	_ = windows.CloseHandle(h)
	_ = cmd.Wait()
}
