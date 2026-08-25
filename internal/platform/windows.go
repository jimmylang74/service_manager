//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type windowsManager struct{}

// New returns a Windows-specific ServiceManager.
func New() ServiceManager {
	return &windowsManager{}
}

func (w *windowsManager) configStorePath(name string) string {
	return filepath.Join(os.Getenv("ProgramData"), name+"-service-manager.conf")
}

func (w *windowsManager) Register(name, displayName, description string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable: %w", err)
	}
	absExe, _ := filepath.Abs(exe)
	// Use sc.exe to register the service
	cmd := fmt.Sprintf(`sc.exe create %s binPath= "%s" start= auto DisplayName= "%s"`, name, absExe, displayName)
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return fmt.Errorf("failed to build sc command")
	}
	return runCommand(parts[0], parts[1:]...)
}

func (w *windowsManager) Unregister(name string) error {
	_ = runCommand("sc.exe", "stop", name)
	return runCommand("sc.exe", "delete", name)
}

func (w *windowsManager) IsRegistered(name string) (bool, error) {
	err := runCommand("sc.exe", "query", name)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "1060") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (w *windowsManager) Run(name string, handler func() error) error {
	// In SCM mode, we'd normally use golang.org/x/sys/windows/svc.
	// For simplicity in the initial implementation, we run the handler directly.
	// A production implementation would integrate with Windows SCM via mcrixt/servicemgr or x/sys.
	return handler()
}

func (w *windowsManager) SetConfigPath(name, path string) error {
	return os.WriteFile(w.configStorePath(name), []byte(path), 0644)
}

func (w *windowsManager) GetConfigPath(name string) (string, error) {
	data, err := os.ReadFile(w.configStorePath(name))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
