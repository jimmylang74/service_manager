package process

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"service-manager/internal/config"
	"service-manager/internal/logger"
)

// Status represents the state of a managed process.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusRunning  Status = "running"
	StatusStarting Status = "starting"
	StatusError    Status = "error"
)

// ManagedProcess wraps an os/exec.Cmd with restart logic and log capture.
type ManagedProcess struct {
	mu       sync.Mutex
	cfg      config.ServiceConfig
	cmd      *exec.Cmd
	status   Status
	writer   *logger.ServiceWriter
	stopCh   chan struct{}
	retries  int
	started  time.Time
}

// New creates a new ManagedProcess.
func New(cfg config.ServiceConfig, logDir string) (*ManagedProcess, error) {
	w, err := logger.NewServiceWriter(logDir, cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("create log writer for %s: %w", cfg.Name, err)
	}
	return &ManagedProcess{
		cfg:    cfg,
		status: StatusStopped,
		writer: w,
		stopCh: make(chan struct{}),
	}, nil
}

// Start launches the process and begins the supervisor loop.
func (mp *ManagedProcess) Start() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.status == StatusRunning {
		return fmt.Errorf("service %s already running", mp.cfg.Name)
	}
	return mp.startLocked()
}

func (mp *ManagedProcess) startLocked() error {
	mp.status = StatusStarting
	cmd := exec.Command(mp.cfg.Executable, mp.cfg.Arguments...)
	cmd.Dir = mp.cfg.WorkingDirectory
	cmd.Stdout = mp.writer
	cmd.Stderr = mp.writer
	mp.cmd = cmd
	if err := cmd.Start(); err != nil {
		mp.status = StatusError
		return fmt.Errorf("start %s: %w", mp.cfg.Name, err)
	}
	mp.status = StatusRunning
	mp.started = time.Now()
	mp.retries = 0
	go mp.supervise()
	return nil
}

// Stop terminates the process.
func (mp *ManagedProcess) Stop() error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.status != StatusRunning && mp.status != StatusStarting {
		return nil
	}
	close(mp.stopCh)
	mp.stopCh = make(chan struct{})
	if mp.cmd != nil && mp.cmd.Process != nil {
		if err := mp.cmd.Process.Kill(); err != nil {
			mp.status = StatusError
			return err
		}
		_ = mp.cmd.Wait()
	}
	mp.status = StatusStopped
	return nil
}

// Restart stops then starts the process.
func (mp *ManagedProcess) Restart() error {
	if err := mp.Stop(); err != nil {
		return err
	}
	return mp.Start()
}

// Status returns current status.
func (mp *ManagedProcess) Status() Status {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.status
}

// Config returns the service config.
func (mp *ManagedProcess) Config() config.ServiceConfig { return mp.cfg }

// StartTime returns when the process was last started.
func (mp *ManagedProcess) StartTime() time.Time {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.started
}

// Pid returns the process ID, or 0 if not running.
func (mp *ManagedProcess) Pid() int {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if mp.cmd != nil && mp.cmd.Process != nil {
		return mp.cmd.Process.Pid
	}
	return 0
}

// supervise watches the process and restarts it according to the restart policy.
func (mp *ManagedProcess) supervise() {
	err := mp.cmd.Wait()
	mp.mu.Lock()
	mp.status = StatusStopped
	policy := mp.cfg.Restart
	mp.mu.Unlock()

	if policy.Policy == "no" {
		return
	}

	if err == nil && policy.Policy != "always" {
		return
	}

	for {
		select {
		case <-mp.stopCh:
			return
		default:
		}
		mp.mu.Lock()
		mp.retries++
		retries := mp.retries
		maxRetries := policy.MaxRetries
		delay := time.Duration(policy.DelaySec) * time.Second
		mp.mu.Unlock()

		if maxRetries > 0 && retries > maxRetries {
			mp.mu.Lock()
			mp.status = StatusError
			mp.mu.Unlock()
			return
		}
		time.Sleep(delay)
		mp.mu.Lock()
		if err := mp.startLocked(); err != nil {
			mp.mu.Unlock()
			continue
		}
		mp.mu.Unlock()
		err = mp.cmd.Wait()
		mp.mu.Lock()
		mp.status = StatusStopped
		mp.mu.Unlock()
		if policy.Policy != "always" {
			return
		}
	}
}

// Close cleans up resources.
func (mp *ManagedProcess) Close() error {
	_ = mp.Stop()
	return mp.writer.Close()
}

func (mp *ManagedProcess) LogFilePath() string {
	return mp.writer.FilePath()
}

// LogTail returns the last n lines from the service log.
func (mp *ManagedProcess) LogTail(n int) ([]string, error) {
	// Read from the log file
	path := mp.writer.FilePath()
	return tailFile(path, n)
}

// LogStream returns an io.Reader that yields new log lines via a pipe.
func (mp *ManagedProcess) LogStream() (io.Reader, func()) {
	pr, pw := io.Pipe()
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			default:
				n, err := mp.writer.File().Read(buf)
				if n > 0 {
					pw.Write(buf[:n])
				}
				if err != nil {
					time.Sleep(500 * time.Millisecond)
				}
			}
		}
	}()
	return pr, func() { pr.Close() }
}
