package streams

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// availablePhysicalOutputs stores which physical DAC outputs (ch0-ch3) exist.
// Set by the stream manager during initialization.
var (
	physOutputsMu            sync.RWMutex
	availablePhysicalOutputs = []int{0} // default: ch0 only (HiFiBerry)
)

// SetAvailablePhysicalOutputs configures which physical DAC outputs (ch0-ch3) are available.
// Called by the stream manager during initialization with data from the hardware profile.
func SetAvailablePhysicalOutputs(outputs []int) {
	physOutputsMu.Lock()
	availablePhysicalOutputs = make([]int, len(outputs))
	copy(availablePhysicalOutputs, outputs)
	physOutputsMu.Unlock()
	slog.Info("alsaloop: available physical outputs configured", "outputs", outputs)
}

// physicalOutputs returns a copy of the configured physical outputs.
func physicalOutputs() []int {
	physOutputsMu.RLock()
	defer physOutputsMu.RUnlock()
	return append([]int(nil), availablePhysicalOutputs...)
}

// isPhysicalOutputAvailable checks if a physical source has a corresponding ALSA device.
func isPhysicalOutputAvailable(physSrc int) bool {
	for _, available := range physicalOutputs() {
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

// ErrPhysicalOutputUnavailable is returned when a stream is routed to a source
// that has no DAC behind it (ch1-ch3 on units without the USB DAC).
var ErrPhysicalOutputUnavailable = errors.New("physical output not available")

// NewALSALoop creates a new ALSALoop that will bridge vsrc to physSrc.
// It fails if physSrc has no ALSA physical output on this unit: audio is never
// rerouted to another DAC, because the zones listening to physSrc would not
// hear it there.
func NewALSALoop(vsrc, physSrc int) (*ALSALoop, error) {
	if !isPhysicalOutputAvailable(physSrc) {
		err := fmt.Errorf("alsaloop: source %d (ch%d): %w; available outputs %v",
			physSrc, physSrc, ErrPhysicalOutputUnavailable, physicalOutputs())
		slog.Error("alsaloop: refusing to route to a missing DAC",
			"vsrc", vsrc, "requested", physSrc, "available", physicalOutputs())
		return nil, err
	}

	a := &ALSALoop{
		vsrc:    vsrc,
		physSrc: physSrc,
	}
	capture := VirtualCaptureDevice(vsrc)
	playback := PhysicalOutputDevice(physSrc)

	a.sup = NewSupervisor(fmt.Sprintf("alsaloop/v%d->ch%d", vsrc, physSrc), func() *exec.Cmd {
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
