package streams

import (
	"context"
	"log/slog"
	"os/exec"
	"syscall"
	"time"
)

// ALSALoop supervises an alsaloop process that bridges vsrc → physSrc.
// Restarts on crash with exponential backoff.
type ALSALoop struct {
	vsrc    int
	physSrc int
	sup     *Supervisor
}

// NewALSALoop creates a new ALSALoop that will bridge vsrc to physSrc.
func NewALSALoop(vsrc, physSrc int) *ALSALoop {
	a := &ALSALoop{
		vsrc:    vsrc,
		physSrc: physSrc,
	}
	capture := VirtualCaptureDevice(vsrc)
	playback := PhysicalOutputDevice(physSrc)

	a.sup = NewSupervisor("alsaloop", func() *exec.Cmd {
		cmd := exec.Command(findBinary("alsaloop"),
			"-C", capture,
			"-P", playback,
			"-t", "100000",
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	// Tune the alsaloop-specific backoff
	a.sup.maxFails = 10
	a.sup.fastFailSec = 3.0
	a.sup.backoff = 500 * time.Millisecond
	a.sup.maxBackoff = 30 * time.Second

	return a
}

// Start begins the alsaloop supervisor goroutine.
func (a *ALSALoop) Start(ctx context.Context) error {
	slog.Info("alsaloop: starting", "vsrc", a.vsrc, "physSrc", a.physSrc)
	return a.sup.Start(ctx)
}

// Stop terminates the alsaloop and waits for the goroutine to exit.
func (a *ALSALoop) Stop() error {
	slog.Info("alsaloop: stopping", "vsrc", a.vsrc, "physSrc", a.physSrc)
	return a.sup.Stop()
}
