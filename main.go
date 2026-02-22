package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/db"
	"github.com/forcedesk/forcedesk-agent/internal/logger"
	"github.com/forcedesk/forcedesk-agent/internal/svc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// First-run setup: prompt for configuration values interactively on all platforms.
	// Skip interactive setup if running as a Windows Service.
	if !config.Exists() && !svc.IsWindowsService() {
		cfg, err = config.Setup()
		if err != nil {
			fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
			os.Exit(1)
		}
	}

	dataDir := config.DataDir()

	// Configure logging mode based on runtime environment.
	// Console logging is enabled when running interactively (not as a service).
	// Non-Windows platforms always use verbose logging for development convenience.
	nonWindows := runtime.GOOS != "windows"
	isService := svc.IsWindowsService()
	consoleMode := nonWindows || !isService
	verbose := nonWindows || (!isService && len(os.Args) > 1 && os.Args[1] == "debug")

	if err := logger.Init(dataDir, consoleMode, verbose); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}

	slog.Info("forcedesk-agent starting",
		"config", config.ConfigPath(),
		"data_dir", dataDir,
		"tenant_url", cfg.Tenant.URL,
	)

	if err := db.Open(dataDir); err != nil {
		slog.Error("failed to open database", "err", err)
		os.Exit(1)
	}

	// If running as a Windows Service, hand off control to the Service Control Manager.
	if isService {
		if err := svc.RunService(); err != nil {
			slog.Error("service exited with error", "err", err)
			os.Exit(1)
		}
		return
	}

	// When invoked without arguments, run the scheduler in the foreground.
	if len(os.Args) < 2 {
		slog.Info("running in foreground — press Ctrl+C to stop")
		svc.RunScheduler()
		return
	}

	switch os.Args[1] {
	case "install":
		if err := svc.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service installed successfully.")

	case "uninstall":
		if err := svc.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service uninstalled successfully.")

	case "start":
		if err := svc.StartService(); err != nil {
			fmt.Fprintf(os.Stderr, "start failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service started.")

	case "stop":
		if err := svc.StopService(); err != nil {
			fmt.Fprintf(os.Stderr, "stop failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service stopped.")

	case "status":
		fmt.Println("Service status:", svc.ServiceStatus())

	case "debug":
		slog.Info("running in debug mode")
		svc.RunScheduler()

	default:
		fmt.Printf("Usage: %s [install|uninstall|start|stop|status|debug]\n", os.Args[0])
		fmt.Println()
		fmt.Println("  install    Register as a Windows Service (auto-start)")
		fmt.Println("  uninstall  Remove the Windows Service")
		fmt.Println("  start      Start the service")
		fmt.Println("  stop       Stop the service")
		fmt.Println("  status     Print the current service status")
		fmt.Println("  debug      Run the scheduler in the foreground with verbose logging")
		fmt.Println()
		fmt.Println("Running without arguments starts the scheduler in the foreground.")
		os.Exit(1)
	}
}
