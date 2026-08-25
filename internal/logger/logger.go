package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager writes manager-level logs to both stderr and the manager log file.
type Manager struct {
	mu   sync.Mutex
	file *os.File
	l    *log.Logger
}

// NewManager creates a logger that writes to logs/manager.log.
func NewManager(logDir string) (*Manager, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(logDir, "manager.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	multi := io.MultiWriter(os.Stderr, f)
	l := log.New(multi, "", log.LstdFlags)
	return &Manager{file: f, l: l}, nil
}

func (m *Manager) Info(format string, args ...any)  { m.output("INFO", format, args...) }
func (m *Manager) Error(format string, args ...any) { m.output("ERROR", format, args...) }
func (m *Manager) Warn(format string, args ...any)  { m.output("WARN", format, args...) }

func (m *Manager) output(level, format string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.l.Printf("[%s] %s", level, fmt.Sprintf(format, args...))
}

func (m *Manager) Close() error {
	if m.file != nil {
		return m.file.Close()
	}
	return nil
}

func (m *Manager) LogPath() string {
	if m.file != nil {
		return m.file.Name()
	}
	return ""
}

// ServiceWriter captures stdout/stderr of a managed service process.
type ServiceWriter struct {
	mu   sync.Mutex
	file *os.File
	name string
}

// NewServiceWriter creates a log file for a named service.
func NewServiceWriter(logDir, serviceName string) (*ServiceWriter, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(logDir, serviceName+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &ServiceWriter{file: f, name: serviceName}, nil
}

// Write implements io.Writer and is safe for goroutine use.
func (w *ServiceWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", ts, w.name, string(p))
	_, _ = w.file.WriteString(line)
	return len(p), nil
}

func (w *ServiceWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *ServiceWriter) FilePath() string {
	if w.file != nil {
		return w.file.Name()
	}
	return ""
}

func (w *ServiceWriter) File() *os.File {
	return w.file
}
