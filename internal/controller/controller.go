// Package controller implements the AmpliPi state machine — the single source
// of truth for all sources, zones, groups, streams, and presets.
package controller

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
	"github.com/micro-nova/amplipi-go/internal/streams"
)

// streamSyncCoalesceDelay is how long to wait after a state change before
// syncing streams. This allows multiple rapid changes to be coalesced.
const streamSyncCoalesceDelay = 50 * time.Millisecond

// Controller is the central state machine for AmpliPi.
// All state mutations go through the apply() method which ensures
// atomicity, persistence, and event publishing.
type Controller struct {
	mu      sync.RWMutex
	state   models.State
	hw      hardware.Driver
	profile *hardware.HardwareProfile // may be nil (no capability restrictions)
	store   config.Store
	bus     *events.Bus
	streams *streams.Manager

	// Stream sync coalescing
	syncMu       sync.Mutex
	syncPending  bool
	syncTimer    *time.Timer
	syncStreams  []models.Stream
	syncSources  []models.Source
	syncShutdown chan struct{}
	syncWg       sync.WaitGroup // Track in-flight sync operations
}

// New creates and initializes a new Controller.
// Loads state from the store and applies it to hardware.
// profile may be nil (no hardware capability restrictions — used in tests).
// mgr may be nil (streams subsystem disabled).
func New(hw hardware.Driver, profile *hardware.HardwareProfile, store config.Store, bus *events.Bus, mgr *streams.Manager) (*Controller, error) {
	state, err := store.Load()
	if err != nil {
		return nil, err
	}

	c := &Controller{
		state:        *state,
		hw:           hw,
		profile:      profile,
		store:        store,
		bus:          bus,
		streams:      mgr,
		syncShutdown: make(chan struct{}),
	}

	// Apply initial state to hardware
	ctx := context.Background()
	if err := c.applyStateToHW(ctx, *state); err != nil {
		// Not fatal — we can run without hardware (mock or debug mode)
		slog.Warn("controller: initial hardware sync failed", "err", err)
	}

	// Sync initial stream state if manager is available
	if c.streams != nil {
		if err := c.streams.Sync(ctx, state.Streams, state.Sources); err != nil {
			// Not fatal — log and continue
			slog.Warn("controller: initial stream sync failed", "err", err)
		}
	}

	return c, nil
}

// Shutdown stops the controller and cleans up resources.
// Blocks until all in-flight sync operations complete.
func (c *Controller) Shutdown() {
	close(c.syncShutdown)
	c.syncMu.Lock()
	if c.syncTimer != nil {
		c.syncTimer.Stop()
	}
	c.syncMu.Unlock()

	// Wait for any in-flight sync operations to complete
	c.syncWg.Wait()
}

// UpdateStreamInfo updates a stream's metadata. Called by the stream Manager
// when a stream's playback state changes.
func (c *Controller) UpdateStreamInfo(id int, info models.StreamInfo) {
	_, _ = c.apply(func(s *models.State) error {
		for i := range s.Streams {
			if s.Streams[i].ID == id {
				s.Streams[i].Info = info
				return nil
			}
		}
		return nil
	})
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
//  4. If fn succeeds: updates state, schedules save, publishes event, syncs streams
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

	// Schedule coalesced stream sync (non-blocking)
	if c.streams != nil {
		c.scheduleStreamSync(next.Streams, next.Sources)
	}

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

// scheduleStreamSync schedules a coalesced stream sync.
// Multiple rapid calls will be coalesced into a single sync operation.
// This prevents goroutine accumulation during rapid state changes.
func (c *Controller) scheduleStreamSync(streams []models.Stream, sources []models.Source) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()

	// Store the latest state to sync
	c.syncStreams = streams
	c.syncSources = sources

	// If a sync is already pending, the timer will use the latest state
	if c.syncPending {
		return
	}

	c.syncPending = true
	c.syncWg.Add(1) // Track the pending callback
	c.syncTimer = time.AfterFunc(streamSyncCoalesceDelay, func() {
		defer c.syncWg.Done()
		c.executeStreamSync()
	})
}

// executeStreamSync performs the actual stream sync operation.
func (c *Controller) executeStreamSync() {
	c.syncMu.Lock()
	streams := c.syncStreams
	sources := c.syncSources
	c.syncPending = false
	c.syncTimer = nil
	c.syncMu.Unlock()

	// Check if we're shutting down
	select {
	case <-c.syncShutdown:
		return
	default:
	}

	if err := c.streams.Sync(context.Background(), streams, sources); err != nil {
		slog.Warn("controller: stream sync failed", "err", err)
	}
}
