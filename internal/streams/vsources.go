package streams

import (
	"errors"
	"fmt"
	"sync"
)

// MaxVSRC is the number of ALSA loopback virtual source slots.
const MaxVSRC = 12

// ErrNoVSRC is returned when no virtual source slots are available.
var ErrNoVSRC = errors.New("no virtual source slots available")

// VSRCAllocator manages a pool of ALSA loopback virtual source indices.
type VSRCAllocator struct {
	mu   sync.Mutex
	used [MaxVSRC]bool
}

// NewVSRCAllocator creates a new VSRCAllocator with all slots free.
func NewVSRCAllocator() *VSRCAllocator {
	return &VSRCAllocator{}
}

// Alloc returns the next free vsrc index, or ErrNoVSRC if all are taken.
func (v *VSRCAllocator) Alloc() (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := 0; i < MaxVSRC; i++ {
		if !v.used[i] {
			v.used[i] = true
			return i, nil
		}
	}
	return -1, ErrNoVSRC
}

// Free releases a vsrc index back to the pool.
func (v *VSRCAllocator) Free(vsrc int) {
	if vsrc < 0 || vsrc >= MaxVSRC {
		return
	}
	v.mu.Lock()
	v.used[vsrc] = false
	v.mu.Unlock()
}

// VirtualCaptureDevice returns the ALSA PCM name for reading from this vsrc.
// Format: "lb{vsrc}p" — the playback side of the loopback (what the stream outputs).
func VirtualCaptureDevice(vsrc int) string {
	return fmt.Sprintf("lb%dp", vsrc)
}

// VirtualOutputDevice returns the ALSA PCM name for writing to this vsrc.
// Format: "lb{vsrc}c" — the capture side of the loopback (where stream audio enters).
func VirtualOutputDevice(vsrc int) string {
	return fmt.Sprintf("lb%dc", vsrc)
}

// PhysicalOutputDevice returns the ALSA PCM name for a physical source.
// Format: "ch{physSrc}" — ch0=HifiBerry, ch1-ch3=USB DAC channels.
func PhysicalOutputDevice(physSrc int) string {
	return fmt.Sprintf("ch%d", physSrc)
}
