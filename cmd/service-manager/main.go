package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"service-manager/internal/api"
	"service-manager/internal/config"
	"service-manager/internal/manager"
	"service-manager/internal/platform"
)

const defaultConfigContent = `web_port: 7070
log_dir: ./logs

services: []
`

const (
	serviceName        = "service-manager"
	serviceDisplayName = "Service Manager"
	serviceDescription = "Cross-platform service management daemon"
)

func main() {
	var (
		action    string
		configPtr string
		portPtr   int
	)

	flag.StringVar(&action, "action", "", "register|uninstall|status")
	flag.StringVar(&configPtr, "config", "", "path to config file")
	flag.IntVar(&portPtr, "port", 0, "override web port")
	flag.Usage = printUsage
	flag.Parse()

	if configPtr == "" {
		configPtr = defaultConfigPath()
	}

	switch action {
	case "register":
		handleRegister(configPtr)
	case "uninstall":
		handleUninstall()
	case "status":
		handleStatus()
	default:
		runDaemon(configPtr, portPtr)
	}
}

func runDaemon(configPath string, portOverride int) {
	ensureConfig(configPath)

	pm := platform.New()
	registered, _ := pm.IsRegistered(serviceName)
	if !registered {
		fmt.Println("first run detected, registering as system service...")
		if err := pm.Register(serviceName, serviceDisplayName, serviceDescription); err != nil {
			fmt.Fprintf(os.Stderr, "auto-register failed: %v\n", err)
		} else {
			_ = pm.SetConfigPath(serviceName, configPath)
			fmt.Printf("service '%s' registered\n", serviceName)
			if err := pm.Start(serviceName); err != nil {
				fmt.Fprintf(os.Stderr, "auto-start failed: %v\n", err)
			} else {
				fmt.Printf("service '%s' started\n", serviceName)
			}
			fmt.Println("this process will now exit; manage via web UI or 'sc' commands")
			return
		}
	}

	loader := config.NewLoader(configPath)
	mgr, err := manager.New(loader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize: %v\n", err)
		os.Exit(1)
	}

	cfg := loader.Get()
	port := cfg.WebPort
	if portOverride > 0 {
		port = portOverride
	}

	mgr.StartFileWatcher()
	if err := mgr.Start(); err != nil {
		mgr.Logger().Error("start failed: %v", err)
		os.Exit(1)
	}

	srv := api.New(mgr, fmt.Sprintf(":%d", port))

	if ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port)); err != nil {
		fmt.Fprintf(os.Stderr, "port %d is already in use - service-manager may already be running\n", port)
		os.Exit(1)
	} else {
		ln.Close()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			mgr.Logger().Error("web server: %v", err)
		}
	}()

	<-sigCh
	fmt.Println("\nshutting down...")
	mgr.StopFileWatcher()
	srv.Stop()
	mgr.Stop()
}

func handleRegister(configPath string) {
	pm := platform.New()
	registered, _ := pm.IsRegistered(serviceName)
	if registered {
		fmt.Println("service already registered, updating config path...")
		if err := pm.SetConfigPath(serviceName, configPath); err != nil {
			fmt.Fprintf(os.Stderr, "failed to update config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("config path updated")
		return
	}
	if err := pm.Register(serviceName, serviceDisplayName, serviceDescription); err != nil {
		fmt.Fprintf(os.Stderr, "failed to register: %v\n", err)
		os.Exit(1)
	}
	if err := pm.SetConfigPath(serviceName, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service '%s' registered successfully\n", serviceName)
}

func handleUninstall() {
	pm := platform.New()
	if err := pm.Unregister(serviceName); err != nil {
		fmt.Fprintf(os.Stderr, "failed to unregister: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service '%s' unregistered\n", serviceName)
}

func handleStatus() {
	pm := platform.New()
	registered, _ := pm.IsRegistered(serviceName)
	if registered {
		fmt.Printf("service '%s' is registered\n", serviceName)
	} else {
		fmt.Printf("service '%s' is not registered\n", serviceName)
	}
}

func defaultConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(filepath.Dir(exe), "config.yaml")
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [options]

Options:
  -action string    Action: register, uninstall, status (default: run daemon)
  -config string    Path to config file (default: <exe_dir>/config.yaml)
  -port int         Override web server port

Actions:
  register          Register as system service (Windows SCM / Linux systemd)
  uninstall         Unregister system service
  status            Show service registration status
  (no action)       Run as daemon with web UI (auto-registers on first run)

Examples:
  %s -action register -config /path/to/config.yaml
  %s -port 8080
  %s -action uninstall
`, os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func ensureConfig(path string) {
	if _, err := os.Stat(path); err == nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create config directory: %v\n", err)
		return
	}
	if err := os.WriteFile(path, []byte(defaultConfigContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot create config file: %v\n", err)
		return
	}
	fmt.Printf("created default config: %s\n", path)
}
