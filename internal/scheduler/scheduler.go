package scheduler

import (
	"log/slog"
	"sync"
	"time"
)

// Task is a named function that runs on a fixed interval.
type Task struct {
	Name     string
	Interval time.Duration
	Fn       func()

	mu sync.Mutex // Prevents overlapping executions of the same task.
}

// Scheduler runs a set of Tasks on fixed intervals.
type Scheduler struct {
	tasks    []*Task
	stopCh   chan struct{}
	tickerWg sync.WaitGroup // Tracks one goroutine per task ticker loop.
	taskWg   sync.WaitGroup // Tracks all running task invocations.
}

// New creates an empty Scheduler.
func New() *Scheduler {
	return &Scheduler{
		stopCh: make(chan struct{}),
	}
}

// Add registers a task. Must be called before Start.
func (s *Scheduler) Add(t *Task) {
	s.tasks = append(s.tasks, t)
}

// Start launches the ticker loops for all registered tasks.
// Each task also fires once immediately on start.
func (s *Scheduler) Start() {
	for _, t := range s.tasks {
		s.tickerWg.Add(1)
		go s.loop(t)
	}
}

// Stop signals all ticker loops to exit and waits for all in-flight task invocations to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.tickerWg.Wait()
	s.taskWg.Wait()
}

// loop manages a single task's ticker loop, firing it immediately and then on each interval.
func (s *Scheduler) loop(t *Task) {
	defer s.tickerWg.Done()

	// Run the task immediately on start.
	s.dispatch(t)

	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.dispatch(t)
		case <-s.stopCh:
			return
		}
	}
}

// dispatch fires the task in a new goroutine, but only if the previous invocation has finished.
// This prevents overlapping executions of the same task.
func (s *Scheduler) dispatch(t *Task) {
	if !t.mu.TryLock() {
		slog.Info("scheduler: skipping task, previous run still in progress", "task", t.Name)
		return
	}

	s.taskWg.Add(1)
	go func() {
		defer s.taskWg.Done()
		defer t.mu.Unlock()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduler: task panicked", "task", t.Name, "panic", r)
			}
		}()

		slog.Info("scheduler: running task", "task", t.Name)
		t.Fn()
		slog.Info("scheduler: task finished", "task", t.Name)
	}()
}
