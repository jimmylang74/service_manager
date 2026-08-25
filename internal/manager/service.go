package manager

import (
	"time"

	"service-manager/internal/config"
)

// ServiceStatus represents the status of a managed service for API responses.
type ServiceStatus struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Pid       int    `json:"pid"`
	StartTime string `json:"start_time,omitempty"`
	Uptime    string `json:"uptime,omitempty"`
}

// ServiceInfo returns status information for a single service.
func (m *Manager) ServiceInfo(name string) (*ServiceStatus, bool) {
	mp, ok := m.Supervisor().Get(name)
	if !ok {
		return nil, false
	}
	info := &ServiceStatus{
		Name:   name,
		Status: string(mp.Status()),
		Pid:    mp.Pid(),
	}
	if mp.Status() == "running" {
		info.StartTime = mp.StartTime().Format("2006-01-02 15:04:05")
		info.Uptime = mp.StartTime().Round(time.Second).String()
	}
	return info, true
}

// AllServiceStatus returns status of all managed services.
func (m *Manager) AllServiceStatus() []ServiceStatus {
	var result []ServiceStatus
	for name := range m.Supervisor().List() {
		if info, ok := m.ServiceInfo(name); ok {
			result = append(result, *info)
		}
	}
	return result
}

// ConfigServices returns the raw service configs.
func (m *Manager) ConfigServices() []config.ServiceConfig {
	cfg := m.Loader().Get()
	if cfg == nil {
		return nil
	}
	return cfg.Services
}
