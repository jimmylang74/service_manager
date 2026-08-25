package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Watch polls the config file for changes and reloads when the modification
// time changes. It blocks until the stop channel is closed.
func (l *Loader) Watch(stop <-chan struct{}, interval time.Duration) {
	var lastMod time.Time
	if info, err := os.Stat(l.path); err == nil {
		lastMod = info.ModTime()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			info, err := os.Stat(l.path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMod) {
				lastMod = info.ModTime()
				if _, err := l.Load(); err != nil {
					fmt.Fprintf(os.Stderr, "config reload error: %v\n", err)
				}
			}
		}
	}
}

// ConfigPath returns the default config path next to the executable.
func ConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(filepath.Dir(exe), "config.yaml")
}

// ServicesDir returns the services/ directory next to the executable.
func ServicesDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "services"
	}
	return filepath.Join(filepath.Dir(exe), "services")
}
