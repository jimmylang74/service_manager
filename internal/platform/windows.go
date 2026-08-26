//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	return runCommand("sc.exe", "create", name,
		"binPath=", absExe,
		"start=", "auto",
		"DisplayName=", displayName,
	)
}

func (w *windowsManager) Unregister(name string) error {
	runCommand("sc.exe", "config", name, "start=", "disabled")
	runCommand("sc.exe", "stop", name)
	time.Sleep(2 * time.Second)

	killOtherProcesses()
	time.Sleep(1 * time.Second)

	for i := 0; i < 10; i++ {
		err := runCommand("sc.exe", "delete", name)
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "1072") {
			killOtherProcesses()
			time.Sleep(2 * time.Second)
			continue
		}
		if strings.Contains(err.Error(), "1060") {
			return nil
		}
		return err
	}
	return nil
}

func killOtherProcesses() {
	myPid := os.Getpid()
	cmd := exec.Command("wmic", "process", "where",
		fmt.Sprintf("name='service-manager.exe' and processid<>%d", myPid),
		"call", "terminate")
	cmd.Run()
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
	return handler()
}

func (w *windowsManager) Start(name string) error {
	return runCommand("sc.exe", "start", name)
}

func (w *windowsManager) Stop(name string) error {
	return runCommand("sc.exe", "stop", name)
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
