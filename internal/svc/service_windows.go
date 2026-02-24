package svc

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/forcedesk/forcedesk-agent/internal/scheduler"
	"github.com/forcedesk/forcedesk-agent/internal/tasks"
)

const serviceName = "ForceDeskAgent"
const serviceDisplay = "ForceDesk Agent"
const serviceDesc = "Monitors and manages the ForceDesk school network agent."

// IsWindowsService reports whether the process is running as a Windows Service.
func IsWindowsService() bool {
	ok, _ := svc.IsWindowsService()
	return ok
}

// RunService starts the Windows Service Control Manager dispatcher.
// This call blocks until the service is stopped by the SCM.
func RunService() error {
	return svc.Run(serviceName, &agentService{})
}

// RunScheduler starts the task scheduler directly in foreground mode (debug/console mode).
// This function blocks until the process is terminated.
func RunScheduler() {
	s := buildScheduler()
	s.Start()
	slog.Info("scheduler running in console mode — press Ctrl+C to stop")
	// Block forever; signal handling is managed by the caller.
	select {}
}

// Install registers the binary as a Windows Service set to start automatically.
func Install() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	// Check if the service is already installed.
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %q already exists", serviceName)
	}

	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName:      serviceDisplay,
		Description:      serviceDesc,
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "LocalSystem",
	})
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	slog.Info("service installed", "name", serviceName, "path", exePath)
	return nil
}

// Uninstall stops (if running) and removes the Windows Service.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	// Attempt to stop the service gracefully before deletion.
	_ = stopService(s)

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}

	slog.Info("service uninstalled", "name", serviceName)
	return nil
}

// StartService asks the SCM to start the service.
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	return s.Start()
}

// StopService asks the SCM to stop the service.
func StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	return stopService(s)
}

// ServiceStatus returns the current service state string.
func ServiceStatus() string {
	m, err := mgr.Connect()
	if err != nil {
		return "unknown"
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return "not installed"
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return "unknown"
	}

	switch status.State {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	case svc.Paused:
		return "paused"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	default:
		return "unknown"
	}
}

// stopService sends a stop signal to the service and waits for it to stop.
// Polls the service status for up to 10 seconds.
func stopService(s *mgr.Service) error {
	_, err := s.Control(svc.Stop)
	if err != nil {
		return err
	}
	// Wait up to 10 seconds for the service to stop.
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		status, err := s.Query()
		if err != nil || status.State == svc.Stopped {
			return err
		}
	}
	return nil
}

// agentService implements the Windows Service handler interface.

type agentService struct{}

func (a *agentService) Execute(
	_ []string,
	req <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	s := buildScheduler()
	s.Start()
	slog.Info("service started", "name", serviceName)

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	for c := range req {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			slog.Info("service stopping")
			s.Stop()
			slog.Info("service stopped")
			return false, 0
		}
	}
	return false, 0
}

// buildScheduler constructs and configures the task scheduler with all agent tasks.
func buildScheduler() *scheduler.Scheduler {
	s := scheduler.New()

	s.Add(&scheduler.Task{
		Name:     "heartbeat",
		Interval: 5 * time.Minute,
		Fn:       tasks.Heartbeat,
	})
	s.Add(&scheduler.Task{
		Name:     "monitoring",
		Interval: 1 * time.Minute,
		Fn:       tasks.MonitoringService,
	})
	s.Add(&scheduler.Task{
		Name:     "devicemanager",
		Interval: 1 * time.Minute,
		Fn:       tasks.DeviceManagerService,
	})
	s.Add(&scheduler.Task{
		Name:     "commandqueue",
		Interval: 1 * time.Minute,
		Fn:       tasks.CommandQueueService,
	})
	s.Add(&scheduler.Task{
		Name:     "devicequery",
		Interval: 5 * time.Second,
		Fn:       tasks.DeviceManagerQuery,
	})
	s.Add(&scheduler.Task{
		Name:     "papercut",
		Interval: 30 * time.Minute,
		Fn:       tasks.PapercutService,
	})
	//s.Add(&scheduler.Task{
	//	Name:     "edustar",
	//	Interval: 4 * time.Hour,
	//	Fn:       tasks.EduStarService,
	//})
	s.Add(&scheduler.Task{
		Name:     "kiosklabel",
		Interval: 10 * time.Second,
		Fn:       tasks.KioskLabelService,
	})

	return s
}
