//go:build !windows

package svc

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/forcedesk/forcedesk-agent/internal/scheduler"
	"github.com/forcedesk/forcedesk-agent/internal/tasks"
)

// IsWindowsService always returns false on non-Windows platforms.
func IsWindowsService() bool { return false }

// RunService is not supported outside Windows.
func RunService() error { return fmt.Errorf("Windows service not supported on this platform") }

// RunScheduler starts the task scheduler directly (for development/testing).
func RunScheduler() {
	s := buildScheduler()
	s.Start()
	slog.Info("scheduler running — press Ctrl+C to stop")
	select {}
}

func Install() error   { return fmt.Errorf("not supported on this platform") }
func Uninstall() error { return fmt.Errorf("not supported on this platform") }
func StartService() error { return fmt.Errorf("not supported on this platform") }
func StopService() error  { return fmt.Errorf("not supported on this platform") }
func ServiceStatus() string { return "n/a" }

func buildScheduler() *scheduler.Scheduler {
	s := scheduler.New()
	s.Add(&scheduler.Task{Name: "heartbeat", Interval: 5 * time.Minute, Fn: tasks.Heartbeat})
	s.Add(&scheduler.Task{Name: "monitoring", Interval: 1 * time.Minute, Fn: tasks.MonitoringService})
	s.Add(&scheduler.Task{Name: "devicemanager", Interval: 1 * time.Minute, Fn: tasks.DeviceManagerService})
	s.Add(&scheduler.Task{Name: "commandqueue", Interval: 1 * time.Minute, Fn: tasks.CommandQueueService})
	s.Add(&scheduler.Task{Name: "devicequery", Interval: 5 * time.Second, Fn: tasks.DeviceManagerQuery})
	s.Add(&scheduler.Task{Name: "papercut", Interval: 30 * time.Minute, Fn: tasks.PapercutService})
	return s
}
