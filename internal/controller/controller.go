// Package controller implements the AmpliPi state machine — the single source
// of truth for all sources, zones, groups, streams, and presets.
package controller

import (
	"context"
	"sync"

	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// Controller is the central state machine for AmpliPi.
// All state mutations go through the apply() method which ensures
// atomicity, persistence, and event publishing.
type Controller struct {
	mu    sync.RWMutex
	state models.State
	hw    hardware.Driver
	store config.Store
	bus   *events.Bus
}

// New creates and initializes a new Controller.
// Loads state from the store and applies it to hardware.
func New(hw hardware.Driver, store config.Store, bus *events.Bus) (*Controller, error) {
	state, err := store.Load()
	if err != nil {
		return nil, err
	}

	c := &Controller{
		state: *state,
		hw:    hw,
		store: store,
		bus:   bus,
	}

	// Apply initial state to hardware
	ctx := context.Background()
	if err := c.applyStateToHW(ctx, *state); err != nil {
		// Not fatal — we can run without hardware (mock or debug mode)
		_ = err
	}

	return c, nil
}

// State returns a deep copy of the current system state.
func (c *Controller) State() models.State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.DeepCopy()
}

// apply is the core mutation primitive. It:
//  1. Acquires the write lock
//  2. Makes a deep copy of current state
//  3. Calls fn to modify the copy (fn may return an error to abort)
//  4. If fn succeeds: updates state, schedules save, publishes event
func (c *Controller) apply(fn func(*models.State) error) (models.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	next := c.state.DeepCopy()
	if err := fn(&next); err != nil {
		return models.State{}, err
	}

	c.state = next
	_ = c.store.Save(&c.state) // debounced, async
	c.bus.Publish(c.state)
	return c.state, nil
}

// applyStateToHW writes the complete state to the hardware driver.
// Called at startup and after factory reset.
func (c *Controller) applyStateToHW(ctx context.Context, state models.State) error {
	for _, unit := range c.hw.Units() {
		// Determine source types (analog/digital) for this unit
		// For simplicity, assume all sources are digital initially
		var analog [4]bool // false = digital
		if err := c.hw.SetSourceTypes(ctx, unit, analog); err != nil {
			return err
		}

		// Configure zones
		baseZone := unit * 6
		var sources [6]int
		var mutes [6]bool
		var enables [6]bool

		for i := 0; i < 6; i++ {
			zoneIdx := baseZone + i
			if zoneIdx < len(state.Zones) {
				z := state.Zones[zoneIdx]
				if z.SourceID >= 0 && z.SourceID <= 3 {
					sources[i] = z.SourceID
				}
				mutes[i] = z.Mute
				enables[i] = !z.Disabled
			} else {
				mutes[i] = true
				enables[i] = false
			}
		}

		if err := c.hw.SetZoneSources(ctx, unit, sources); err != nil {
			return err
		}
		if err := c.hw.SetZoneMutes(ctx, unit, mutes); err != nil {
			return err
		}
		if err := c.hw.SetAmpEnables(ctx, unit, enables); err != nil {
			return err
		}

		// Set volumes
		for i := 0; i < 6; i++ {
			zoneIdx := baseZone + i
			if zoneIdx < len(state.Zones) {
				vol := state.Zones[zoneIdx].Vol
				if err := c.hw.SetZoneVol(ctx, unit, i, vol); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// findZone returns a pointer to the zone with the given ID in the state, or nil.
func findZone(state *models.State, id int) *models.Zone {
	for i := range state.Zones {
		if state.Zones[i].ID == id {
			return &state.Zones[i]
		}
	}
	return nil
}

// findGroup returns a pointer to the group with the given ID, or nil.
func findGroup(state *models.State, id int) *models.Group {
	for i := range state.Groups {
		if state.Groups[i].ID == id {
			return &state.Groups[i]
		}
	}
	return nil
}

// findStream returns a pointer to the stream with the given ID, or nil.
func findStream(state *models.State, id int) *models.Stream {
	for i := range state.Streams {
		if state.Streams[i].ID == id {
			return &state.Streams[i]
		}
	}
	return nil
}

// findPreset returns a pointer to the preset with the given ID, or nil.
func findPreset(state *models.State, id int) *models.Preset {
	for i := range state.Presets {
		if state.Presets[i].ID == id {
			return &state.Presets[i]
		}
	}
	return nil
}

// nextGroupID returns the next available group ID.
func nextGroupID(state *models.State) int {
	maxID := 99
	for _, g := range state.Groups {
		if g.ID > maxID {
			maxID = g.ID
		}
	}
	return maxID + 1
}

// nextStreamID returns the next available stream ID.
func nextStreamID(state *models.State) int {
	maxID := 999
	for _, s := range state.Streams {
		if s.ID > maxID {
			maxID = s.ID
		}
	}
	return maxID + 1
}

// nextPresetID returns the next available preset ID.
func nextPresetID(state *models.State) int {
	maxID := 0
	for _, p := range state.Presets {
		if p.ID > maxID && p.ID < models.LastPresetID {
			maxID = p.ID
		}
	}
	return maxID + 1
}
