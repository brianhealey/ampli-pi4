package streams

import (
	"context"
	"log/slog"
	"os/exec"
	"syscall"
	"time"
)

// availablePhysicalOutputs stores which physical DAC outputs (ch0-ch3) exist.
// Set by the stream manager during initialization.
var availablePhysicalOutputs = []int{0} // default: ch0 only (HiFiBerry)

// SetAvailablePhysicalOutputs configures which physical DAC outputs (ch0-ch3) are available.
// Called by the stream manager during initialization with data from the hardware profile.
func SetAvailablePhysicalOutputs(outputs []int) {
	availablePhysicalOutputs = make([]int, len(outputs))
	copy(availablePhysicalOutputs, outputs)
	slog.Info("alsaloop: available physical outputs configured", "outputs", availablePhysicalOutputs)
}

// isPhysicalOutputAvailable checks if a physical source has a corresponding ALSA device.
func isPhysicalOutputAvailable(physSrc int) bool {
	for _, available := range availablePhysicalOutputs {
		if available == physSrc {
			return true
		}
	}
	return false
}

// ALSALoop supervises an alsaloop process that bridges vsrc → physSrc.
// Restarts on crash with exponential backoff.
type ALSALoop struct {
	vsrc    int
	physSrc int
	sup     *Supervisor
}

// NewALSALoop creates a new ALSALoop that will bridge vsrc to physSrc.
// On v1 hardware (without USB DAC), falls back to ch0 for all sources,
// allowing ALSA's dmix to mix multiple streams together.
func NewALSALoop(vsrc, physSrc int) (*ALSALoop, error) {
	// Fall back to ch0 if requested physical output doesn't exist (v1 hardware behavior)
	actualPhysSrc := physSrc
	if !isPhysicalOutputAvailable(physSrc) {
		slog.Warn("alsaloop: physical output not available, falling back to ch0",
			"requested", physSrc, "available", availablePhysicalOutputs)
		actualPhysSrc = 0 // Fall back to HiFiBerry DAC (uses dmix for multiple streams)
	}

	a := &ALSALoop{
		vsrc:    vsrc,
		physSrc: actualPhysSrc,
	}
	capture := VirtualCaptureDevice(vsrc)
	playback := PhysicalOutputDevice(actualPhysSrc)

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

	return a, nil
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
