package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// FMRadioStream receives FM radio via rtl_fm and pipes it to ALSA via aplay.
// Non-persistent. Only one FMRadio stream may be active at a time (RTL-SDR
// is exclusive-access hardware). The Manager enforces this constraint.
type FMRadioStream struct {
	name string
	freq string // e.g. "96.5M"

	mu    sync.Mutex
	rtl   *exec.Cmd
	aplay *exec.Cmd
	loop  *ALSALoop
	vsrc  int
	done  chan struct{}

	info   models.StreamInfo
	infoMu sync.RWMutex
}

// NewFMRadioStream creates a new FM radio stream.
func NewFMRadioStream(name, freq string) *FMRadioStream {
	return &FMRadioStream{
		name: name,
		freq: freq,
	}
}

// Activate starts the rtl_fm → aplay pipeline.
func (s *FMRadioStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("fm_radio: activating", "name", s.name, "freq", s.freq)

	if _, err := buildConfigDir(configDir, vsrc); err != nil {
		return fmt.Errorf("fm_radio activate: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rtl != nil {
		return fmt.Errorf("fm_radio: already active")
	}

	device := VirtualOutputDevice(vsrc)
	freq := s.freq

	// Build rtl_fm command
	rtlCmd := exec.Command(findBinary("rtl_fm"),
		"-f", freq,
		"-M", "fm",
		"-s", "200k",
		"-r", "44100",
		"-",
	)
	rtlCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Build aplay command
	aplayCmd := exec.Command(findBinary("aplay"),
		"-r", "44100",
		"-f", "S16_LE",
		"-t", "raw",
		"-D", device,
	)
	aplayCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Pipe rtl_fm stdout → aplay stdin
	pipe, err := rtlCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("fm_radio: stdout pipe: %w", err)
	}
	aplayCmd.Stdin = pipe

	if err := rtlCmd.Start(); err != nil {
		return fmt.Errorf("fm_radio: start rtl_fm: %w", err)
	}
	if err := aplayCmd.Start(); err != nil {
		_ = rtlCmd.Process.Kill()
		return fmt.Errorf("fm_radio: start aplay: %w", err)
	}

	s.rtl = rtlCmd
	s.aplay = aplayCmd
	s.vsrc = vsrc
	s.done = make(chan struct{})

	// Goroutine to wait on both processes
	go func() {
		defer close(s.done)
		_ = rtlCmd.Wait()
		_ = aplayCmd.Wait()
		slog.Info("fm_radio: pipeline exited", "name", s.name)
		s.infoMu.Lock()
		s.info.State = "stopped"
		s.infoMu.Unlock()
	}()

	s.infoMu.Lock()
	s.info = models.StreamInfo{Name: s.name, State: "playing"}
	s.infoMu.Unlock()

	return nil
}

// Deactivate kills the rtl_fm and aplay processes.
func (s *FMRadioStream) Deactivate(ctx context.Context) error {
	slog.Info("fm_radio: deactivating", "name", s.name)

	s.mu.Lock()
	rtl := s.rtl
	aplay := s.aplay
	loop := s.loop
	done := s.done
	s.rtl = nil
	s.aplay = nil
	s.loop = nil
	s.mu.Unlock()

	if loop != nil {
		_ = loop.Stop()
	}

	if rtl != nil && rtl.Process != nil {
		_ = syscall.Kill(-rtl.Process.Pid, syscall.SIGTERM)
	}
	if aplay != nil && aplay.Process != nil {
		_ = syscall.Kill(-aplay.Process.Pid, syscall.SIGTERM)
	}

	if done != nil {
		<-done
	}
	return nil
}

// Connect starts the ALSA loop bridge.
func (s *FMRadioStream) Connect(ctx context.Context, physSrc int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loop != nil {
		_ = s.loop.Stop()
	}
	loop, err := NewALSALoop(s.vsrc, physSrc)
	if err != nil {
		return fmt.Errorf("alsaloop creation failed: %w", err)
	}
	s.loop = loop
	return s.loop.Start(ctx)
}

// Disconnect stops the ALSA loop bridge.
func (s *FMRadioStream) Disconnect(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loop != nil {
		err := s.loop.Stop()
		s.loop = nil
		return err
	}
	return nil
}

func (s *FMRadioStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("fm_radio: command ignored", "name", s.name, "cmd", cmd)
	return nil
}

func (s *FMRadioStream) Info() models.StreamInfo {
	s.infoMu.RLock()
	defer s.infoMu.RUnlock()
	return s.info
}

func (s *FMRadioStream) IsPersistent() bool { return false }
func (s *FMRadioStream) Type() string        { return "fm_radio" }
