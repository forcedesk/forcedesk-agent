// Copyright © 2026 ForcePoint Software. All rights reserved.

package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// TaskState is a point-in-time snapshot of a task's runtime state,
// returned by Scheduler.States for the WebUI status API.
type TaskState struct {
	Name      string     `json:"name"`
	Interval  string     `json:"interval"`
	LastRun   *time.Time `json:"last_run"`
	LastEnd   *time.Time `json:"last_end"`
	Duration  string     `json:"duration"`
	NextRun   *time.Time `json:"next_run"`
	Running   bool       `json:"running"`
	RunCount  int64      `json:"run_count"`
	LastPanic string     `json:"last_panic"`
}

// Task is a named function that runs on a fixed interval.
type Task struct {
	Name     string
	Interval time.Duration
	Fn       func()

	mu sync.Mutex // Prevents overlapping executions of the same task.

	// stateMu guards the fields below; separate from mu so the WebUI can read
	// state while a task is mid-execution without deadlocking.
	stateMu   sync.RWMutex
	lastRun   time.Time
	lastEnd   time.Time
	nextRun   time.Time
	running   bool
	runCount  int64
	lastPanic string
}

// Scheduler runs a set of Tasks on fixed intervals.
type Scheduler struct {
	tasks     []*Task
	stopCh    chan struct{}
	tickerWg  sync.WaitGroup // Tracks one goroutine per task ticker loop.
	taskWg    sync.WaitGroup // Tracks all running task invocations.
	startedAt time.Time
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
	s.startedAt = time.Now()
	for _, t := range s.tasks {
		s.tickerWg.Add(1)
		go s.loop(t)
	}
}

// StartedAt returns the time the scheduler was started.
func (s *Scheduler) StartedAt() time.Time {
	return s.startedAt
}

// Stop signals all ticker loops to exit and waits for all in-flight task invocations to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.tickerWg.Wait()
	s.taskWg.Wait()
}

// States returns a snapshot of each registered task's current state.
// Safe to call concurrently with running tasks.
func (s *Scheduler) States() []TaskState {
	states := make([]TaskState, len(s.tasks))
	for i, t := range s.tasks {
		t.stateMu.RLock()

		state := TaskState{
			Name:      t.Name,
			Interval:  t.Interval.String(),
			Running:   t.running,
			RunCount:  t.runCount,
			LastPanic: t.lastPanic,
		}

		if !t.lastRun.IsZero() {
			lr := t.lastRun
			state.LastRun = &lr
		}
		if !t.lastEnd.IsZero() {
			le := t.lastEnd
			state.LastEnd = &le
			if !t.lastRun.IsZero() {
				// Round to milliseconds to keep the string readable.
				state.Duration = t.lastEnd.Sub(t.lastRun).Round(time.Millisecond).String()
			}
		}
		if !t.nextRun.IsZero() {
			nr := t.nextRun
			state.NextRun = &nr
		}

		t.stateMu.RUnlock()
		states[i] = state
	}
	return states
}

// loop manages a single task's ticker loop, firing it immediately and then on each interval.
func (s *Scheduler) loop(t *Task) {
	defer s.tickerWg.Done()

	// The first run fires immediately; the next tick arrives one Interval later.
	t.stateMu.Lock()
	t.nextRun = time.Now().Add(t.Interval)
	t.stateMu.Unlock()

	s.dispatch(t)

	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()

	for {
		select {
		case tick := <-ticker.C:
			// Compute the next expected tick before dispatching so the UI always
			// shows a non-zero next-run time even while the task is executing.
			t.stateMu.Lock()
			t.nextRun = tick.Add(t.Interval)
			t.stateMu.Unlock()

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

	// Record the start time and mark the task as running before launching the goroutine
	// so the WebUI reflects an accurate state immediately.
	now := time.Now()
	t.stateMu.Lock()
	t.lastRun = now
	t.running = true
	t.stateMu.Unlock()

	s.taskWg.Add(1)
	go func() {
		defer s.taskWg.Done()
		defer t.mu.Unlock()

		// Record end time and clear running flag unconditionally; this deferred
		// func runs even if the task panics (the recover below runs first).
		defer func() {
			end := time.Now()
			t.stateMu.Lock()
			t.lastEnd = end
			t.running = false
			t.runCount++
			t.stateMu.Unlock()
		}()

		// Catch panics so a misbehaving task can't crash the scheduler process.
		// The panic message is surfaced in the WebUI for easy diagnosis.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("scheduler: task panicked", "task", t.Name, "panic", r)
				t.stateMu.Lock()
				t.lastPanic = fmt.Sprintf("%v", r)
				t.stateMu.Unlock()
			}
		}()

		slog.Info("scheduler: running task", "task", t.Name)
		t.Fn()
		slog.Info("scheduler: task finished", "task", t.Name)
	}()
}
