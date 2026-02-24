package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/forcedesk/forcedesk-agent/internal/config"
	"github.com/forcedesk/forcedesk-agent/internal/db"
	"github.com/forcedesk/forcedesk-agent/internal/logger"
	"github.com/forcedesk/forcedesk-agent/internal/svc"
	"github.com/forcedesk/forcedesk-agent/internal/tasks"
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

	case "edustar":
		if len(os.Args) < 3 {
			printEduStarUsage(os.Args[0])
			os.Exit(1)
		}
		action := os.Args[2]

		fs := flag.NewFlagSet("edustar", flag.ExitOnError)
		fs.Usage = func() { printEduStarUsage(os.Args[0]) }
		school := fs.String("school", "", "School ID (overrides configured school code)")
		dn := fs.String("dn", "", "User or student DN (for user, reset-password, set-password)")
		groupDN := fs.String("group-dn", "", "Group DN (for group, add-to-group, remove-from-group)")
		groupName := fs.String("group-name", "", "Group name (for group)")
		memberDN := fs.String("member-dn", "", "Member DN (for add-to-group, remove-from-group)")
		newPassword := fs.String("new-password", "", "New password (for set-password)")
		if err := fs.Parse(os.Args[3:]); err != nil {
			os.Exit(1)
		}

		tasks.RunEduStarCLI(action, tasks.EduStarCLIOpts{
			School:      *school,
			DN:          *dn,
			GroupDN:     *groupDN,
			GroupName:   *groupName,
			MemberDN:    *memberDN,
			NewPassword: *newPassword,
		})

	default:
		fmt.Printf("Usage: %s [install|uninstall|start|stop|status|debug|edustar]\n", os.Args[0])
		fmt.Println()
		fmt.Println("  install    Register as a Windows Service (auto-start)")
		fmt.Println("  uninstall  Remove the Windows Service")
		fmt.Println("  start      Start the service")
		fmt.Println("  stop       Stop the service")
		fmt.Println("  status     Print the current service status")
		fmt.Println("  debug      Run the scheduler in the foreground with verbose logging")
		fmt.Println("  edustar    Run an EduStar STMC action and print the output")
		fmt.Println()
		fmt.Println("Running without arguments starts the scheduler in the foreground.")
		os.Exit(1)
	}
}

func printEduStarUsage(exe string) {
	fmt.Fprintf(os.Stderr, "Usage: %s edustar <action> [flags]\n", exe)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Actions:")
	fmt.Fprintln(os.Stderr, "  whoami                     Get authenticated user info")
	fmt.Fprintln(os.Stderr, "  schools                    List available schools")
	fmt.Fprintln(os.Stderr, "  all-schools                List all school IDs")
	fmt.Fprintln(os.Stderr, "  students                   List students for the configured school")
	fmt.Fprintln(os.Stderr, "  staff                      List staff and technicians")
	fmt.Fprintln(os.Stderr, "  technicians                List technicians only")
	fmt.Fprintln(os.Stderr, "  groups                     List groups for the configured school")
	fmt.Fprintln(os.Stderr, "  group                      Get group members  (--group-dn, --group-name)")
	fmt.Fprintln(os.Stderr, "  certificates               List certificates")
	fmt.Fprintln(os.Stderr, "  service-accounts           List service accounts")
	fmt.Fprintln(os.Stderr, "  nps                        Get NPS mapping")
	fmt.Fprintln(os.Stderr, "  user                       Get user by TO number/alias  (--dn)")
	fmt.Fprintln(os.Stderr, "  reset-password             Reset student password  (--dn)")
	fmt.Fprintln(os.Stderr, "  set-password               Set student password  (--dn, --new-password)")
	fmt.Fprintln(os.Stderr, "  add-to-group               Add member to group  (--group-dn, --member-dn)")
	fmt.Fprintln(os.Stderr, "  remove-from-group          Remove member from group  (--group-dn, --member-dn)")
	fmt.Fprintln(os.Stderr, "  populate-student-accounts  Sync student accounts to tenant")
	fmt.Fprintln(os.Stderr, "  populate-staff-accounts    Sync staff accounts to tenant")
	fmt.Fprintln(os.Stderr, "  populate-crt-accounts      Sync CRT accounts to tenant")
	fmt.Fprintln(os.Stderr, "  expire-crt-accounts        Disable CRT accounts and scramble passwords")
	fmt.Fprintln(os.Stderr, "  enable-crt-accounts        Enable CRT accounts and set daily passwords")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --school <id>        Override the configured school code")
	fmt.Fprintln(os.Stderr, "  --dn <dn>            User/student distinguished name")
	fmt.Fprintln(os.Stderr, "  --group-dn <dn>      Group distinguished name")
	fmt.Fprintln(os.Stderr, "  --group-name <name>  Group name")
	fmt.Fprintln(os.Stderr, "  --member-dn <dn>     Member distinguished name")
	fmt.Fprintln(os.Stderr, "  --new-password <pw>  New password for set-password")
}
