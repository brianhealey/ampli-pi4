package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
	"github.com/micro-nova/amplipi-go/internal/streams"
)

const (
	// ANNOUNCE_PRESET_ID is the fixed ID for the temporary announcement preset
	ANNOUNCE_PRESET_ID = 9998
	// ANNOUNCE_RESTORE_PRESET_ID is the fixed ID for the state save preset
	ANNOUNCE_RESTORE_PRESET_ID = 9999
	// ANNOUNCE_POLL_INTERVAL is how often we check if the announcement has finished
	ANNOUNCE_POLL_INTERVAL = 100 * time.Millisecond
	// ANNOUNCE_MAX_DURATION is the maximum time we'll wait for an announcement to complete
	ANNOUNCE_MAX_DURATION = 10 * time.Minute
)

// Announce creates a PA-style announcement that:
// 1. Saves current state
// 2. Creates a temporary fileplayer stream with the media URL
// 3. Creates a temporary preset connecting target zones to the announcement
// 4. Waits for the announcement to finish playing (blocking)
// 5. Cleans up temporary resources and restores previous state
//
// This operation blocks until the announcement completes or times out.
func (c *Controller) Announce(ctx context.Context, req models.AnnounceRequest) (models.State, *models.AppError) {
	// Validate request
	if req.Media == "" {
		return models.State{}, models.ErrBadRequest("media URL is required")
	}

	// Validate media URL/path to prevent command injection
	if err := streams.ValidateFilePath(req.Media); err != nil {
		return models.State{}, models.ErrBadRequest(fmt.Sprintf("invalid media: %v", err))
	}

	// Set defaults
	sourceID := 3 // default to source 3
	if req.SourceID != nil {
		sourceID = *req.SourceID
	} else if !c.profile.SourceHasDAC(sourceID) {
		// The announcement is a digital stream: default to the highest source
		// that has a DAC (source 0 on units without the USB DAC).
		if digital := c.profile.DigitalSources(); len(digital) > 0 {
			sourceID = digital[len(digital)-1]
		}
	}
	if sourceID < 0 || sourceID >= models.MaxSources {
		return models.State{}, models.ErrBadRequest(fmt.Sprintf("source_id must be 0-%d", models.MaxSources-1))
	}
	if !c.profile.SourceHasDAC(sourceID) {
		return models.State{}, models.ErrBadRequest(noDACMessage(c.profile, sourceID))
	}

	volF := 0.5 // default to 50% relative volume
	if req.VolF != nil {
		volF = *req.VolF
		if volF < 0.0 || volF > 1.0 {
			return models.State{}, models.ErrBadRequest("vol_f must be between 0.0 and 1.0")
		}
	}

	// Step 1: Read current state (single lock acquisition to avoid deadlock)
	c.mu.RLock()
	currentState := c.state.DeepCopy()
	c.mu.RUnlock()

	// Step 1b: Save current state to a restore preset (uses the pre-read state)
	saveState, err := c.saveCurrentStateFromCopy(ctx, currentState)
	if err != nil {
		return models.State{}, err
	}

	// Step 2: Create temporary fileplayer stream
	streamID, err := c.createAnnouncementStream(ctx, req.Media)
	if err != nil {
		// Try to restore state before returning error
		_, _ = c.restoreStateAndCleanup(ctx, saveState, 0)
		return models.State{}, err
	}

	// Step 3: Determine target zones (use pre-read state to avoid nested locks)
	targetZones, err := c.determineTargetZonesFromState(currentState, req.Zones, req.Groups)
	if err != nil {
		// Cleanup stream and restore state
		_, _ = c.restoreStateAndCleanup(ctx, saveState, streamID)
		return models.State{}, err
	}

	// Step 4: Create and load announcement preset (use pre-read state to avoid nested locks)
	announcementState, err := c.createAndLoadAnnouncementPreset(ctx, currentState, sourceID, streamID, targetZones, req.Vol, volF)
	if err != nil {
		// Cleanup stream and restore state
		_, _ = c.restoreStateAndCleanup(ctx, saveState, streamID)
		return models.State{}, err
	}

	// Step 5: Wait for announcement to finish (poll stream state)
	if err := c.waitForAnnouncementToFinish(ctx, streamID); err != nil {
		// Cleanup and restore even on timeout/error
		_, _ = c.restoreStateAndCleanup(ctx, saveState, streamID)
		return models.State{}, err
	}

	// Step 6: Cleanup and restore previous state
	finalState, err := c.restoreStateAndCleanup(ctx, saveState, streamID)
	if err != nil {
		return announcementState, err // return announcement state if we can't restore
	}

	return finalState, nil
}

// saveCurrentStateFromCopy captures the provided system state in a preset for later restoration.
// This avoids acquiring locks when called from Announce() which already read the state.
func (c *Controller) saveCurrentStateFromCopy(ctx context.Context, currentState models.State) (models.State, *models.AppError) {
	// Build a preset that captures current source, zone, and group state
	var sourceUpdates []models.SourceUpdate
	for _, src := range currentState.Sources {
		id := src.ID
		name := src.Name
		input := src.Input
		sourceUpdates = append(sourceUpdates, models.SourceUpdate{
			ID:    &id,
			Name:  &name,
			Input: &input,
		})
	}

	var zoneUpdates []models.ZoneUpdate
	for _, z := range currentState.Zones {
		id := z.ID
		name := z.Name
		sourceID := z.SourceID
		mute := z.Mute
		vol := z.Vol
		volF := z.VolF
		volMin := z.VolMin
		volMax := z.VolMax
		disabled := z.Disabled
		zoneUpdates = append(zoneUpdates, models.ZoneUpdate{
			ID:       &id,
			Name:     &name,
			SourceID: &sourceID,
			Mute:     &mute,
			Vol:      &vol,
			VolF:     &volF,
			VolMin:   &volMin,
			VolMax:   &volMax,
			Disabled: &disabled,
		})
	}

	var groupUpdates []models.GroupUpdate
	for _, g := range currentState.Groups {
		id := g.ID
		name := g.Name
		zones := make([]int, len(g.ZoneIDs))
		copy(zones, g.ZoneIDs)
		groupUpdates = append(groupUpdates, models.GroupUpdate{
			ID:       &id,
			Name:     &name,
			ZoneIDs:  zones,
			SourceID: g.SourceID,
			Vol:      g.Vol,
			VolF:     g.VolF,
			Mute:     g.Mute,
		})
	}

	presetState := models.PresetState{
		Sources: sourceUpdates,
		Zones:   zoneUpdates,
		Groups:  groupUpdates,
	}

	// Create or update the restore preset
	state, err := c.apply(func(s *models.State) error {
		// Check if restore preset already exists
		existing := findPreset(s, ANNOUNCE_RESTORE_PRESET_ID)
		if existing != nil {
			// Update it
			existing.Name = "PA - Saved State"
			existing.State = &presetState
		} else {
			// Create it
			preset := models.Preset{
				ID:    ANNOUNCE_RESTORE_PRESET_ID,
				Name:  "PA - Saved State",
				State: &presetState,
			}
			s.Presets = append(s.Presets, preset)
		}
		return nil
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}

	return state, nil
}

// createAnnouncementStream creates a temporary fileplayer stream for the announcement
func (c *Controller) createAnnouncementStream(ctx context.Context, mediaURL string) (int, *models.AppError) {
	req := models.StreamCreate{
		Name: "PA - Announcement",
		Type: "file_player",
		Config: map[string]interface{}{
			"path":      mediaURL,
			"temporary": true,
		},
	}

	state, err := c.CreateStream(ctx, req)
	if err != nil {
		return 0, err
	}

	// Find the stream we just created (it will be the last one)
	if len(state.Streams) == 0 {
		return 0, models.ErrInternal("failed to create announcement stream")
	}

	// Find stream by name since we just created it
	var streamID int
	for _, s := range state.Streams {
		if s.Name == "PA - Announcement" {
			streamID = s.ID
			break
		}
	}
	if streamID == 0 {
		return 0, models.ErrInternal("failed to find created announcement stream")
	}

	return streamID, nil
}

// determineTargetZonesFromState resolves the target zones from the zones and groups lists
// using the provided state. If both are empty, returns all enabled zones.
// This avoids acquiring locks when called from Announce() which already read the state.
func (c *Controller) determineTargetZonesFromState(state models.State, zoneIDs, groupIDs []int) ([]int, *models.AppError) {
	targetZones := make(map[int]bool)

	// If both zones and groups are empty, use all enabled zones
	if len(zoneIDs) == 0 && len(groupIDs) == 0 {
		for _, z := range state.Zones {
			if !z.Disabled {
				targetZones[z.ID] = true
			}
		}
	} else {
		// Add explicitly specified zones
		for _, zid := range zoneIDs {
			z := findZone(&state, zid)
			if z != nil && !z.Disabled {
				targetZones[zid] = true
			}
		}

		// Add zones from groups
		for _, gid := range groupIDs {
			g := findGroup(&state, gid)
			if g != nil {
				for _, zid := range g.ZoneIDs {
					z := findZone(&state, zid)
					if z != nil && !z.Disabled {
						targetZones[zid] = true
					}
				}
			}
		}
	}

	// Convert map to slice
	result := make([]int, 0, len(targetZones))
	for zid := range targetZones {
		result = append(result, zid)
	}

	if len(result) == 0 {
		return nil, models.ErrBadRequest("no enabled zones found for announcement")
	}

	return result, nil
}

// createAndLoadAnnouncementPreset creates a preset that configures the announcement
// and immediately loads it. Uses the provided state to avoid acquiring nested locks.
func (c *Controller) createAndLoadAnnouncementPreset(
	ctx context.Context,
	currentState models.State,
	sourceID, streamID int,
	targetZones []int,
	volDB *int,
	volF float64,
) (models.State, *models.AppError) {
	// Build the announcement preset
	sourceInput := fmt.Sprintf("stream=%d", streamID)
	srcID := sourceID
	srcInput := sourceInput
	sourceUpdate := models.SourceUpdate{
		ID:    &srcID,
		Input: &srcInput,
	}

	// Build zone updates for target zones
	var zoneUpdates []models.ZoneUpdate
	for _, zid := range targetZones {
		id := zid
		src := sourceID
		mute := false
		update := models.ZoneUpdate{
			ID:       &id,
			SourceID: &src,
			Mute:     &mute,
		}

		if volDB != nil {
			// Use absolute volume if specified
			vol := *volDB
			update.Vol = &vol
		} else {
			// Use relative volume
			vf := volF
			update.VolF = &vf
		}

		zoneUpdates = append(zoneUpdates, update)
	}

	// Get all zones affected by changing this source (from pre-read state)
	affectedZones := make(map[int]bool)
	for _, z := range currentState.Zones {
		if z.SourceID == sourceID {
			affectedZones[z.ID] = true
		}
	}

	// Mute zones affected by source change but not in announcement
	for zid := range affectedZones {
		inAnnouncement := false
		for _, targetID := range targetZones {
			if zid == targetID {
				inAnnouncement = true
				break
			}
		}
		if !inAnnouncement {
			id := zid
			src := sourceID
			mute := true
			zoneUpdates = append(zoneUpdates, models.ZoneUpdate{
				ID:       &id,
				SourceID: &src,
				Mute:     &mute,
			})
		}
	}

	presetState := models.PresetState{
		Sources: []models.SourceUpdate{sourceUpdate},
		Zones:   zoneUpdates,
	}

	// Create or update the announcement preset
	_, err := c.apply(func(s *models.State) error {
		existing := findPreset(s, ANNOUNCE_PRESET_ID)
		if existing != nil {
			existing.Name = "PA - Active Announcement"
			existing.State = &presetState
		} else {
			preset := models.Preset{
				ID:    ANNOUNCE_PRESET_ID,
				Name:  "PA - Active Announcement",
				State: &presetState,
			}
			s.Presets = append(s.Presets, preset)
		}
		return nil
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}

	// Load the announcement preset
	return c.LoadPreset(ctx, ANNOUNCE_PRESET_ID)
}

// waitForAnnouncementToFinish polls the stream state until it's stopped/disconnected.
// It uses a two-phase approach:
// 1. Wait for stream to start playing (or timeout if it never starts)
// 2. Wait for stream to finish playing
func (c *Controller) waitForAnnouncementToFinish(ctx context.Context, streamID int) *models.AppError {
	deadline := time.Now().Add(ANNOUNCE_MAX_DURATION)
	ticker := time.NewTicker(ANNOUNCE_POLL_INTERVAL)
	defer ticker.Stop()

	// Phase 1: Wait for stream to start playing (max 10 seconds)
	// VLC can take several seconds to start, especially with network URLs
	startDeadline := time.Now().Add(10 * time.Second)
	streamStarted := false

	for !streamStarted {
		select {
		case <-ctx.Done():
			return models.ErrInternal("announcement cancelled")
		case <-ticker.C:
			if time.Now().After(startDeadline) {
				return models.ErrInternal("announcement stream failed to start")
			}

			// Query the stream manager directly for live state
			// (the model state is updated asynchronously and may lag)
			var state string
			if c.streams != nil {
				if info := c.streams.Info(streamID); info != nil {
					state = info.State
				}
			}
			// Fallback to model state if manager unavailable
			if state == "" {
				c.mu.RLock()
				stream := findStream(&c.state, streamID)
				c.mu.RUnlock()
				if stream == nil {
					return models.ErrInternal("announcement stream was deleted before starting")
				}
				state = stream.Info.State
			}

			if state == "playing" || state == "loading" {
				streamStarted = true
			}
		}
	}

	// Phase 2: Wait for stream to finish playing
	for {
		select {
		case <-ctx.Done():
			return models.ErrInternal("announcement cancelled")
		case <-ticker.C:
			if time.Now().After(deadline) {
				return models.ErrInternal("announcement timeout exceeded")
			}

			// Query the stream manager directly for live state
			var state string
			var streamExists bool
			if c.streams != nil {
				if info := c.streams.Info(streamID); info != nil {
					state = info.State
					streamExists = true
				}
			}
			// Fallback to model state if manager unavailable
			if !streamExists {
				c.mu.RLock()
				stream := findStream(&c.state, streamID)
				c.mu.RUnlock()
				if stream == nil {
					// Stream was deleted, announcement is done
					return nil
				}
				state = stream.Info.State
			}

			if state == "stopped" || state == "disconnected" || state == "" {
				// Announcement finished
				return nil
			}
		}
	}
}

// restoreStateAndCleanup restores the saved state and deletes temporary resources
func (c *Controller) restoreStateAndCleanup(ctx context.Context, savedState models.State, streamID int) (models.State, *models.AppError) {
	// Load the restore preset
	state, err := c.LoadPreset(ctx, ANNOUNCE_RESTORE_PRESET_ID)
	if err != nil {
		return savedState, err
	}

	// Delete the announcement preset
	_, _ = c.DeletePreset(ctx, ANNOUNCE_PRESET_ID)

	// Delete the restore preset
	_, _ = c.DeletePreset(ctx, ANNOUNCE_RESTORE_PRESET_ID)

	// Delete the temporary stream
	if streamID != 0 {
		_, _ = c.DeleteStream(ctx, streamID)
	}

	return state, nil
}
