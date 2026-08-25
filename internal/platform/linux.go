//go:build linux

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdUnitFile = `[Unit]
Description=%s
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=always
RestartSec=%d
EnvironmentFile=-%s

[Install]
WantedBy=multi-user.target
`

type linuxManager struct{}

// New returns a Linux-specific ServiceManager.
func New() ServiceManager {
	return &linuxManager{}
}

func (l *linuxManager) unitPath(name string) string {
	return filepath.Join("/etc/systemd/system", name+".service")
}

func (l *linuxManager) configStorePath(name string) string {
	return filepath.Join("/etc", name+"-service-manager.conf")
}

func (l *linuxManager) Register(name, displayName, description string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}
	absExe, _ := filepath.Abs(exe)
	workDir := filepath.Dir(absExe)
	content := fmt.Sprintf(systemdUnitFile, displayName, absExe, workDir, 5, "")
	if err := os.WriteFile(l.unitPath(name), []byte(content), 0644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := exec.Command("systemctl", "enable", name+".service").Run(); err != nil {
		return fmt.Errorf("enable service: %w", err)
	}
	return nil
}

func (l *linuxManager) Unregister(name string) error {
	_ = exec.Command("systemctl", "stop", name+".service").Run()
	_ = exec.Command("systemctl", "disable", name+".service").Run()
	if err := os.Remove(l.unitPath(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove unit file: %w", err)
	}
	os.Remove(l.configStorePath(name))
	return exec.Command("systemctl", "daemon-reload").Run()
}

func (l *linuxManager) IsRegistered(name string) (bool, error) {
	_, err := os.Stat(l.unitPath(name))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (l *linuxManager) Run(name string, handler func() error) error {
	return handler()
}

func (l *linuxManager) Start(name string) error {
	return exec.Command("systemctl", "start", name+".service").Run()
}

func (l *linuxManager) Stop(name string) error {
	return exec.Command("systemctl", "stop", name+".service").Run()
}

func (l *linuxManager) SetConfigPath(name, path string) error {
	return os.WriteFile(l.configStorePath(name), []byte(path), 0644)
}

func (l *linuxManager) GetConfigPath(name string) (string, error) {
	data, err := os.ReadFile(l.configStorePath(name))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
