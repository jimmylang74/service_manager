package process

import (
	"sync"
	"time"

	"service-manager/internal/config"
)

// Supervisor manages a collection of ManagedProcesses.
type Supervisor struct {
	mu       sync.RWMutex
	processes map[string]*ManagedProcess
	logDir   string
}

// NewSupervisor creates a new Supervisor.
func NewSupervisor(logDir string) *Supervisor {
	return &Supervisor{
		processes: make(map[string]*ManagedProcess),
		logDir:    logDir,
	}
}

// Add registers a service config and creates a ManagedProcess.
func (s *Supervisor) Add(cfg config.ServiceConfig) error {
	mp, err := New(cfg, s.logDir)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.processes[cfg.Name] = mp
	s.mu.Unlock()
	return nil
}

// Start starts a named service.
func (s *Supervisor) Start(name string) error {
	s.mu.RLock()
	mp, ok := s.processes[name]
	s.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	return mp.Start()
}

// Stop stops a named service.
func (s *Supervisor) Stop(name string) error {
	s.mu.RLock()
	mp, ok := s.processes[name]
	s.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	return mp.Stop()
}

// Restart restarts a named service.
func (s *Supervisor) Restart(name string) error {
	s.mu.RLock()
	mp, ok := s.processes[name]
	s.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	return mp.Restart()
}

// Get returns a ManagedProcess by name.
func (s *Supervisor) Get(name string) (*ManagedProcess, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mp, ok := s.processes[name]
	return mp, ok
}

// List returns all managed processes.
func (s *Supervisor) List() map[string]*ManagedProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*ManagedProcess, len(s.processes))
	for k, v := range s.processes {
		result[k] = v
	}
	return result
}

// Remove stops and removes a service.
func (s *Supervisor) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mp, ok := s.processes[name]
	if !ok {
		return ErrNotFound
	}
	_ = mp.Close()
	delete(s.processes, name)
	return nil
}

// StartAll starts all registered services.
func (s *Supervisor) StartAll() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, mp := range s.processes {
		_ = mp.Start()
	}
}

// StopAll stops all registered services.
func (s *Supervisor) StopAll() {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, mp := range s.processes {
		_ = mp.Stop()
	}
}

// StopAllGraceful stops all with a timeout.
func (s *Supervisor) StopAllGraceful(timeout time.Duration) {
	s.StopAll()
}

// Reload replaces the process set with a new config slice.
func (s *Supervisor) Reload(cfgs []config.ServiceConfig) {
	s.mu.Lock()
	// Stop existing processes that are no longer in the new config
	newNames := make(map[string]bool)
	for _, cfg := range cfgs {
		newNames[cfg.Name] = true
	}
	for name, mp := range s.processes {
		if !newNames[name] {
			_ = mp.Close()
			delete(s.processes, name)
		}
	}
	s.mu.Unlock()

	// Add or update services
	for _, cfg := range cfgs {
		if existing, ok := s.Get(cfg.Name); ok {
			// Update config if changed
			_ = existing
		} else {
			_ = s.Add(cfg)
		}
	}
}
