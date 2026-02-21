package streams

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultMaxFails    = 5
	defaultFastFailSec = 5.0
	defaultMaxBackoff  = 30 * time.Second
	backoffReset       = 30 * time.Second // reset backoff if process ran this long
	sigtermTimeout     = 3 * time.Second
)

// Supervisor manages a single subprocess with restart logic.
// It is safe to call Start/Stop concurrently.
type Supervisor struct {
	name     string
	buildCmd func() *exec.Cmd

	// Restart policy
	maxFails    int
	fastFailSec float64
	maxBackoff  time.Duration

	// Internal state (protected by mu)
	mu           sync.Mutex
	currentPID   int
	backoff      time.Duration
	failCount    int
	stopCh       chan struct{}
	doneCh       chan struct{}
	running      bool
}

// NewSupervisor creates a Supervisor with sensible defaults.
func NewSupervisor(name string, buildCmd func() *exec.Cmd) *Supervisor {
	return &Supervisor{
		name:        name,
		buildCmd:    buildCmd,
		maxFails:    defaultMaxFails,
		fastFailSec: defaultFastFailSec,
		maxBackoff:  defaultMaxBackoff,
		backoff:     500 * time.Millisecond,
	}
}

// Start launches the subprocess and begins the supervision goroutine.
// ctx cancellation stops supervision and kills the process.
// Returns an error if already running.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.failCount = 0
	s.backoff = 500 * time.Millisecond
	s.running = true
	go s.supervise(ctx)
	return nil
}

// Stop sends a termination signal and waits for the supervisor goroutine to exit.
// Safe to call if not running.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	stopCh := s.stopCh
	doneCh := s.doneCh
	s.mu.Unlock()

	close(stopCh)

	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		slog.Warn("supervisor stop timed out", "name", s.name)
	}
	return nil
}

// Pid returns the current process PID, or 0 if not running.
func (s *Supervisor) Pid() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentPID
}

// supervise runs in a goroutine. It starts the process, waits for it to exit,
// then decides whether to restart.
func (s *Supervisor) supervise(ctx context.Context) {
	defer func() {
		s.mu.Lock()
		s.running = false
		s.currentPID = 0
		doneCh := s.doneCh
		s.mu.Unlock()
		close(doneCh)
	}()

	for {
		// Check if we should stop
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		default:
		}

		// Check fail limit
		s.mu.Lock()
		if s.failCount >= s.maxFails {
			slog.Error("supervisor giving up after too many fast-fails", "name", s.name, "fails", s.failCount)
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		// Build and start the command
		cmd := s.buildCmd()
		if cmd == nil {
			slog.Error("supervisor: buildCmd returned nil", "name", s.name)
			return
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		startTime := time.Now()
		slog.Info("supervisor: starting process", "name", s.name, "cmd", cmd.Path)

		if err := cmd.Start(); err != nil {
			// Binary not found is permanent — no point retrying
			if errors.Is(err, exec.ErrNotFound) || isNotFoundError(err) {
				slog.Error("supervisor: binary not found, giving up", "name", s.name, "cmd", cmd.Path, "err", err)
				return
			}
			slog.Error("supervisor: failed to start process", "name", s.name, "err", err)
			// Count as a fast-fail
			s.mu.Lock()
			s.failCount++
			backoff := s.backoff
			s.backoff = minDuration(s.backoff*2, s.maxBackoff)
			s.mu.Unlock()
			s.sleepOrStop(ctx, backoff)
			continue
		}

		// Record PID
		pid := cmd.Process.Pid
		s.mu.Lock()
		s.currentPID = pid
		s.mu.Unlock()

		slog.Info("supervisor: process running", "name", s.name, "pid", pid)

		// Wait for process to exit in a goroutine so we can also watch stopCh/ctx
		exitCh := make(chan error, 1)
		go func() {
			exitCh <- cmd.Wait()
		}()

		var exitErr error
		select {
		case exitErr = <-exitCh:
		case <-s.stopCh:
			s.killProcess(pid)
			<-exitCh
			return
		case <-ctx.Done():
			s.killProcess(pid)
			<-exitCh
			return
		}

		elapsed := time.Since(startTime)
		slog.Info("supervisor: process exited", "name", s.name, "pid", pid, "elapsed", elapsed, "err", exitErr)

		s.mu.Lock()
		s.currentPID = 0

		if elapsed >= backoffReset {
			// Ran long enough — reset fail tracking and backoff
			s.failCount = 0
			s.backoff = 500 * time.Millisecond
		} else if elapsed.Seconds() < s.fastFailSec {
			s.failCount++
			s.backoff = minDuration(s.backoff*2, s.maxBackoff)
		} else {
			// Moderate failure — don't count as fast-fail but keep backoff
			s.failCount = 0
		}

		backoff := s.backoff
		s.mu.Unlock()

		// Wait before restarting
		if backoff > 0 {
			s.sleepOrStop(ctx, backoff)
		}
	}
}

// killProcess sends SIGTERM to the process group, waits sigtermTimeout,
// then escalates to SIGKILL.
func (s *Supervisor) killProcess(pid int) {
	if pid <= 0 {
		return
	}
	slog.Debug("supervisor: sending SIGTERM to process group", "pid", pid)
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		// Poll until the process group is dead
		deadline := time.Now().Add(sigtermTimeout)
		for time.Now().Before(deadline) {
			if syscall.Kill(-pid, 0) != nil {
				// Process group gone
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(sigtermTimeout + 100*time.Millisecond):
		slog.Warn("supervisor: SIGTERM timed out, sending SIGKILL", "pid", pid)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

// sleepOrStop sleeps for d or returns early if stop/ctx is signalled.
func (s *Supervisor) sleepOrStop(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-s.stopCh:
	case <-ctx.Done():
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// isNotFoundError returns true if err indicates the binary was not found.
// Catches both exec.ErrNotFound and the underlying "no such file or directory" / "executable not found" OS errors.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory") ||
		errors.Is(err, exec.ErrNotFound)
}
