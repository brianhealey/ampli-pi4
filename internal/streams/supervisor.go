package streams

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultMaxFails       = 5
	defaultFastFailSec    = 5.0
	defaultMaxBackoff     = 30 * time.Second
	defaultInitialBackoff = 500 * time.Millisecond
	backoffReset          = 30 * time.Second // reset backoff if process ran this long
	sigtermTimeout        = 3 * time.Second
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
	runOnce     bool        // If true, don't restart after process exits
	onExit      func(error) // Called when process exits (before restart decision)

	// onGiveUp, if set, is called from the supervision goroutine when it stops
	// restarting the process on its own (fail limit reached or binary
	// missing), not when Stop or ctx ends it. It must not call Stop or Start
	// on this Supervisor.
	onGiveUp func(reason string)

	// sleepHook, if set, is told about every backoff sleep (tests).
	sleepHook func(time.Duration)

	// retryForever keeps restarting after maxFails fast failures instead of
	// giving up, with the backoff still capped at maxBackoff. For processes
	// whose failures are usually transient (network not up yet at boot). A
	// missing binary still gives up immediately.
	retryForever bool

	// logStderr forwards the child's stderr to the daemon log (rate-limited,
	// see childLogWriter) when the command does not already set cmd.Stderr.
	// On by default so any child's failure is visible in the journal.
	logStderr bool

	// initialBackoff is the delay before the first restart, and what the
	// backoff returns to after a process has run for backoffReset.
	initialBackoff time.Duration

	// healthCheck, if set, is polled every healthInterval once the process has
	// run for healthGrace. After healthFailures consecutive errors the process
	// group is killed and the exit is treated as a fast failure, so the normal
	// restart and backoff apply. It is for processes that stay alive but stop
	// doing their job (e.g. a player that lost its audio output). It must
	// return nil when health cannot be determined.
	healthCheck    func() error
	healthGrace    time.Duration
	healthInterval time.Duration
	healthFailures int

	// Internal state (protected by mu)
	mu         sync.Mutex
	currentPID int
	backoff    time.Duration
	failCount  int
	stopCh     chan struct{}
	doneCh     chan struct{}
	running    bool
}

// NewSupervisor creates a Supervisor with sensible defaults.
func NewSupervisor(name string, buildCmd func() *exec.Cmd) *Supervisor {
	return &Supervisor{
		name:        name,
		buildCmd:    buildCmd,
		maxFails:    defaultMaxFails,
		fastFailSec: defaultFastFailSec,
		maxBackoff:  defaultMaxBackoff,
		backoff:     defaultInitialBackoff,
		logStderr:   true,

		initialBackoff: defaultInitialBackoff,
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
	s.backoff = s.initialBackoff
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

		// Check fail limit. Reaching it is normally caught right after the
		// failure that reached it (see failedFast); this covers maxFails <= 0.
		s.mu.Lock()
		if s.failCount >= s.maxFails {
			if !s.retryForever {
				fails := s.failCount
				s.mu.Unlock()
				s.giveUpFailing(fails)
				return
			}
			if s.failCount == s.maxFails {
				slog.Warn("supervisor: still failing, will keep retrying",
					"name", s.name, "fails", s.failCount, "max_backoff", s.maxBackoff)
			}
		}
		s.mu.Unlock()

		// Build and start the command
		cmd := s.buildCmd()
		if cmd == nil {
			slog.Error("supervisor: buildCmd returned nil", "name", s.name)
			return
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		var stderrLog *childLogWriter
		if s.logStderr && cmd.Stderr == nil {
			stderrLog = newChildLogWriter(s.name)
			cmd.Stderr = stderrLog
			// A grandchild holding stderr open must not block Wait forever.
			cmd.WaitDelay = 2 * time.Second
		}

		startTime := time.Now()
		slog.Info("supervisor: starting process", "name", s.name, "cmd", cmd.Path)

		if err := cmd.Start(); err != nil {
			// Binary not found is permanent — no point retrying
			if errors.Is(err, exec.ErrNotFound) || isNotFoundError(err) {
				// One ERROR per event: when a hook handles the give-up it
				// logs the outcome itself.
				s.logGiveUp("supervisor: binary not found, giving up", "cmd", cmd.Path, "err", err)
				s.giveUp(fmt.Sprintf("%s not found", cmd.Path))
				return
			}
			slog.Error("supervisor: failed to start process", "name", s.name, "err", err)
			// Count as a fast-fail
			s.mu.Lock()
			s.failCount++
			backoff := s.backoff
			s.backoff = minDuration(s.backoff*2, s.maxBackoff)
			if fails, done := s.exhausted(); done {
				s.mu.Unlock()
				s.giveUpFailing(fails)
				return
			}
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

		var healthTick <-chan time.Time
		var healthTicker *time.Ticker
		if s.healthCheck != nil && s.healthInterval > 0 {
			healthTicker = time.NewTicker(s.healthInterval)
			healthTick = healthTicker.C
		}
		stopHealth := func() {
			if healthTicker != nil {
				healthTicker.Stop()
				healthTick = nil
			}
		}
		// unhealthy: killed by the health check without ever having passed it,
		// i.e. the process never did its job this run.
		unhealthy := false
		sawHealthy := false
		healthErrs := 0

		var exitErr error
	wait:
		for {
			select {
			case exitErr = <-exitCh:
				break wait
			case <-healthTick:
				if time.Since(startTime) < s.healthGrace {
					continue
				}
				err := s.healthCheck()
				if err == nil {
					healthErrs = 0
					sawHealthy = true
					continue
				}
				healthErrs++
				if healthErrs < max(s.healthFailures, 1) {
					continue
				}
				slog.Warn("supervisor: process alive but unhealthy, killing it to restart",
					"name", s.name, "pid", pid, "reason", err, "running_for", time.Since(startTime).Round(time.Second))
				unhealthy = !sawHealthy
				stopHealth()
				s.killProcess(pid)
			case <-s.stopCh:
				s.killProcess(pid)
				// Wait for process exit with timeout to prevent goroutine leak
				select {
				case <-exitCh:
					if stderrLog != nil {
						stderrLog.Close()
					}
				case <-time.After(10 * time.Second):
					slog.Error("supervisor: process won't die, leaking wait goroutine", "name", s.name, "pid", pid)
				}
				stopHealth()
				return
			case <-ctx.Done():
				s.killProcess(pid)
				// Wait for process exit with timeout to prevent goroutine leak
				select {
				case <-exitCh:
					if stderrLog != nil {
						stderrLog.Close()
					}
				case <-time.After(10 * time.Second):
					slog.Error("supervisor: process won't die, leaking wait goroutine", "name", s.name, "pid", pid)
				}
				stopHealth()
				return
			}
		}
		stopHealth()

		elapsed := time.Since(startTime)
		if stderrLog != nil {
			stderrLog.Close()
		}
		slog.Info("supervisor: process exited", "name", s.name, "pid", pid, "elapsed", elapsed, "err", exitErr)

		// Call onExit callback if set
		if s.onExit != nil {
			s.onExit(exitErr)
		}

		// If runOnce mode, don't restart
		if s.runOnce {
			slog.Info("supervisor: runOnce mode, not restarting", "name", s.name)
			return
		}

		s.mu.Lock()
		s.currentPID = 0

		if elapsed >= backoffReset && !unhealthy {
			// Ran long enough — reset fail tracking and backoff
			s.failCount = 0
			s.backoff = s.initialBackoff
		} else if elapsed.Seconds() < s.fastFailSec || unhealthy {
			// Exit status is irrelevant: VLC exits 0 when its URL fails.
			s.failCount++
			s.backoff = minDuration(s.backoff*2, s.maxBackoff)
			// Decide before sleeping: a backoff that is not followed by a
			// restart is just dead time (silence, for an audio bridge).
			if fails, done := s.exhausted(); done {
				s.mu.Unlock()
				s.giveUpFailing(fails)
				return
			}
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

// exhausted reports whether the fail limit is reached and the supervisor
// should stop restarting. Caller holds s.mu.
func (s *Supervisor) exhausted() (fails int, done bool) {
	return s.failCount, !s.retryForever && s.failCount >= s.maxFails
}

// giveUpFailing ends supervision after too many fast failures.
func (s *Supervisor) giveUpFailing(fails int) {
	s.logGiveUp("supervisor giving up after too many fast-fails", "fails", fails)
	s.giveUp(fmt.Sprintf("exited or failed its health check %d times in a row", fails))
}

// logGiveUp logs a give-up at ERROR, or at INFO when an onGiveUp hook will
// report the event (it logs its own ERROR with the consequence).
func (s *Supervisor) logGiveUp(msg string, args ...any) {
	args = append([]any{"name", s.name}, args...)
	if s.onGiveUp != nil {
		slog.Info(msg, args...)
		return
	}
	slog.Error(msg, args...)
}

// giveUp runs the onGiveUp hook, if any.
func (s *Supervisor) giveUp(reason string) {
	if s.onGiveUp != nil {
		s.onGiveUp(reason)
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
	if s.sleepHook != nil {
		s.sleepHook(d)
	}
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
