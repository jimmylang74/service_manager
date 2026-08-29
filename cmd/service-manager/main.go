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
		uninstall bool
		status    bool
		install   bool
		console   bool
		configPtr string
		portPtr   int
	)

	registerSCMFlag()
	flag.BoolVar(&uninstall, "uninstall", false, "unregister and remove the system service")
	flag.BoolVar(&status, "status", false, "show service registration status")
	flag.BoolVar(&install, "install", false, "force re-register the system service")
	flag.BoolVar(&console, "console", false, "keep the console attached (do not detach via FreeConsole)")
	flag.StringVar(&configPtr, "config", "", "path to config file (default: <exe_dir>/config.yaml)")
	flag.IntVar(&portPtr, "port", 0, "override web server port")
	flag.Usage = printUsage
	flag.Parse()

	// --help / -h / -help is handled inside flag.Parse(): usage is printed to
	// the console and the process exits before this point. Detach from the
	// console afterwards so normal runs do not keep a console window open,
	// unless -console asks to keep it attached.
	if !console {
		detachConsole()
	}

	if configPtr == "" {
		configPtr = defaultConfigPath()
	}

	switch {
	case uninstall:
		handleUninstall()
	case status:
		handleStatus()
	case install:
		handleInstall(configPtr)
	case scmOpt != nil && *scmOpt:
		if isSCM() {
			runDaemon(configPtr, portPtr)
		} else {
			handleManualRun(configPtr, portPtr)
		}
	default:
		if isSCM() {
			runDaemon(configPtr, portPtr)
		} else {
			handleDebugRun(configPtr, portPtr)
		}
	}
}

func runDaemon(configPath string, portOverride int) {
	runWithSCM(serviceName, func(stopCh chan struct{}) {
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
					fmt.Println("you can start it manually later")
				} else {
					fmt.Printf("service '%s' started\n", serviceName)
				}
				fmt.Println("this process will now exit; manage via web UI or system service commands")
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
			return
		}

		srv := api.New(mgr, fmt.Sprintf(":%d", port))

		if ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port)); err != nil {
			mgr.Logger().Error("port %d is already in use", port)
			return
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

		select {
		case <-sigCh:
		case <-stopCh:
		}

		mgr.Logger().Info("shutting down...")
		mgr.StopFileWatcher()
		srv.Stop()
		mgr.Stop()
	})
}

func handleInstall(configPath string) {
	ensureConfig(configPath)
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

func handleManualRun(configPath string, portOverride int) {
	ensureConfig(configPath)

	pm := platform.New()
	registered, _ := pm.IsRegistered(serviceName)
	if !registered {
		fmt.Println("first run detected, registering as system service...")
		if err := pm.Register(serviceName, serviceDisplayName, serviceDescription); err != nil {
			fmt.Fprintf(os.Stderr, "auto-register failed: %v\n", err)
			fmt.Println("starting in manual mode (web UI available)")
		} else {
			_ = pm.SetConfigPath(serviceName, configPath)
			fmt.Printf("service '%s' registered\n", serviceName)
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
		return
	}

	srv := api.New(mgr, fmt.Sprintf(":%d", port))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	mgr.Logger().Info("web UI: http://localhost:%d", port)

	go func() {
		if err := srv.Start(); err != nil {
			mgr.Logger().Error("web server: %v", err)
		}
	}()

	<-sigCh

	mgr.Logger().Info("shutting down...")
	mgr.StopFileWatcher()
	srv.Stop()
	mgr.Stop()
}

func handleDebugRun(configPath string, portOverride int) {
	ensureConfig(configPath)

	loader := config.NewLoader(configPath)
	mgr, err := manager.New(loader, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize: %v\n", err)
		os.Exit(1)
	}

	cfg := loader.Get()
	port := cfg.WebPort
	if portOverride > 0 {
		port = portOverride
	}

	mgr.Logger().Info("running directly (no system service registration): config=%s", configPath)
	mgr.Logger().Info("starting services: %d configured", len(cfg.Services))
	mgr.StartFileWatcher()
	if err := mgr.Start(); err != nil {
		mgr.Logger().Error("start failed: %v", err)
		return
	}

	srv := api.New(mgr, fmt.Sprintf(":%d", port))

	if ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port)); err != nil {
		mgr.Logger().Error("port %d is already in use", port)
		return
	} else {
		ln.Close()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	mgr.Logger().Info("web UI: http://localhost:%d", port)

	go func() {
		if err := srv.Start(); err != nil {
			mgr.Logger().Error("web server: %v", err)
		}
	}()

	<-sigCh

	mgr.Logger().Info("shutting down...")
	mgr.StopFileWatcher()
	srv.Stop()
	mgr.Stop()
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
	scmOption, scmBehavior, scmExample := "", "", ""
	if scmOpt != nil {
		scmOption = "  -scm              run as a system service (register and start it when launched under SCM)\n"
		scmBehavior = "  -scm              first run: register the service only\n                    launched by SCM: run the system service\n"
		scmExample = fmt.Sprintf("  %s -scm                     # register and run as a system service\n", os.Args[0])
	}
	fmt.Fprintf(os.Stderr, `Usage: %s [options]

Options:
%s  -console          keep the console attached (do not detach via FreeConsole)
  -install          force re-register the system service
  -uninstall        unregister and remove the system service
  -status           show service registration status
  -config string    path to config file (default: <exe_dir>/config.yaml)
  -port int         override web server port

Behavior:
%s  (no flags)        run directly without registering as a system service (default)
                    launched by systemd/SCM: run the system service
  -console          stay attached to the console so output is visible
  -install          register service without starting it
  -uninstall        stop and remove the service

Examples:
  %s                          # run directly, logs to file
  %s -port 8080               # run directly on a custom port
  %s -config ./cfg.yaml       # run directly with a custom config
%s  %s -console                 # run directly, keep console output visible
`, os.Args[0], scmOption, scmBehavior, os.Args[0], os.Args[0], os.Args[0], scmExample, os.Args[0])
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
