package manager

import (
	"fmt"
	"sync"
	"time"

	"service-manager/internal/config"
	"service-manager/internal/logger"
	"service-manager/internal/process"
)

// Manager orchestrates service lifecycle.
type Manager struct {
	mu         sync.RWMutex
	supervisor *process.Supervisor
	loader     *config.Loader
	logger     *logger.Manager
	stopCh     chan struct{}
}

// New creates a Manager from a config loader.
// If fileOnly is true, logs go only to files (no console output).
func New(loader *config.Loader, fileOnly ...bool) (*Manager, error) {
	cfg := loader.Get()
	if cfg == nil {
		var err error
		cfg, err = loader.Load()
		if err != nil {
			return nil, err
		}
	}
	mgr, err := logger.NewManager(cfg.LogDir, fileOnly...)
	if err != nil {
		return nil, err
	}
	mgr.Info("service manager starting")
	return &Manager{
		supervisor: process.NewSupervisor(cfg.LogDir),
		loader:     loader,
		logger:     mgr,
		stopCh:     make(chan struct{}),
	}, nil
}

// Start loads config and starts all managed services.
func (m *Manager) Start() error {
	cfg := m.loader.Get()
	if cfg == nil {
		return fmt.Errorf("no config loaded")
	}
	for _, svc := range cfg.Services {
		if err := m.supervisor.Add(svc); err != nil {
			m.logger.Error("add service %s: %v", svc.Name, err)
			continue
		}
		m.logger.Info("starting service: %s", svc.Name)
		if err := m.supervisor.Start(svc.Name); err != nil {
			m.logger.Error("start service %s: %v", svc.Name, err)
		}
	}
	m.logger.Info("all services started")
	return nil
}

// Stop stops all services and cleans up.
func (m *Manager) Stop() {
	m.logger.Info("stopping all services")
	m.supervisor.StopAllGraceful(10 * time.Second)
	m.logger.Info("service manager stopped")
	m.logger.Close()
}

// Reload reloads the config and reconciles services.
func (m *Manager) Reload() error {
	cfg, err := m.loader.Load()
	if err != nil {
		return err
	}
	m.supervisor.Reload(cfg.Services)
	m.logger.Info("configuration reloaded")
	return nil
}

// Supervisor returns the underlying process supervisor.
func (m *Manager) Supervisor() *process.Supervisor {
	return m.supervisor
}

// Logger returns the manager logger.
func (m *Manager) Logger() *logger.Manager {
	return m.logger
}

// Loader returns the config loader.
func (m *Manager) Loader() *config.Loader {
	return m.loader
}

// StartFileWatcher starts polling for config changes.
func (m *Manager) StartFileWatcher() {
	go m.loader.Watch(m.stopCh, 2*time.Second)
}

// StopFileWatcher stops the config file watcher.
func (m *Manager) StopFileWatcher() {
	close(m.stopCh)
}
