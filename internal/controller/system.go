package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/identity"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetInfo returns system information, enriched with hardware profile data when available.
func (c *Controller) GetInfo() models.Info {
	info := models.Info{
		Version:  identity.GetVersion(),
		IsUpdate: identity.IsUpdateMode(),
		Offline:  !identity.GetOnlineStatus(),
	}

	// Populate hardware profile fields if a profile is available
	if c.profile != nil {
		info.Units = len(c.profile.Units)
		info.Zones = c.profile.TotalZones
		info.FirmwareVersion = c.profile.FirmwareVersion
		info.FanMode = c.profile.FanMode.String()
		info.AvailableStreams = c.profile.AvailableStreamTypes()
	}

	return info
}

// TestPreamp runs a quick preamp self-test by reading the version registers from all units.
func (c *Controller) TestPreamp(ctx context.Context) (map[string]interface{}, error) {
	if c.hw == nil {
		return map[string]interface{}{"ok": false, "error": "no hardware driver"}, nil
	}

	if c.profile == nil || len(c.profile.Units) == 0 {
		return map[string]interface{}{
			"ok":      true,
			"details": "no preamp units detected (mock mode)",
		}, nil
	}

	results := make([]map[string]interface{}, 0, len(c.profile.Units))
	allOK := true

	for _, unit := range c.profile.Units {
		maj, err := c.hw.Read(ctx, unit.Index, hardware.RegVersionMaj)
		if err != nil {
			allOK = false
			results = append(results, map[string]interface{}{
				"unit":  unit.Index,
				"ok":    false,
				"error": err.Error(),
			})
			continue
		}
		min, err := c.hw.Read(ctx, unit.Index, hardware.RegVersionMin)
		if err != nil {
			allOK = false
			results = append(results, map[string]interface{}{
				"unit":  unit.Index,
				"ok":    false,
				"error": err.Error(),
			})
			continue
		}
		results = append(results, map[string]interface{}{
			"unit":    unit.Index,
			"ok":      true,
			"version": fmt.Sprintf("%d.%d", maj, min),
		})
	}

	return map[string]interface{}{
		"ok":      allOK,
		"details": fmt.Sprintf("tested %d unit(s)", len(c.profile.Units)),
		"units":   results,
	}, nil
}

// TestFans forces fans on for 3 seconds then returns to auto mode.
func (c *Controller) TestFans(ctx context.Context) (map[string]interface{}, error) {
	if c.hw == nil {
		return map[string]interface{}{"ok": false, "error": "no hardware driver"}, nil
	}

	if c.profile == nil || len(c.profile.Units) == 0 {
		return map[string]interface{}{
			"ok":      true,
			"details": "no preamp units detected (mock mode)",
		}, nil
	}

	const fanFullDuty byte = 0xFF // 100% duty cycle
	const fanAutoDuty byte = 0x00 // return to firmware-controlled auto

	// Force fans on for all units
	for _, unit := range c.profile.Units {
		if err := c.hw.Write(ctx, unit.Index, hardware.RegFanDuty, fanFullDuty); err != nil {
			return map[string]interface{}{
				"ok":    false,
				"error": fmt.Sprintf("failed to set fan duty on unit %d: %v", unit.Index, err),
			}, nil
		}
	}

	// Wait 3 seconds (respecting context cancellation)
	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
		return map[string]interface{}{"ok": false, "error": "context cancelled"}, nil
	}

	// Return to auto
	for _, unit := range c.profile.Units {
		_ = c.hw.Write(ctx, unit.Index, hardware.RegFanDuty, fanAutoDuty)
	}

	return map[string]interface{}{
		"ok":      true,
		"details": fmt.Sprintf("fans tested on %d unit(s), returned to auto", len(c.profile.Units)),
	}, nil
}

// FactoryReset resets the system to default state and pushes it to hardware.
func (c *Controller) FactoryReset(ctx context.Context) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		// Preserve the current version info
		info := s.Info
		// Use profile-aware default state if profile is available
		*s = models.DefaultStateFromProfile(c.profile)
		s.Info = info

		// Push to hardware
		return c.applyStateToHW(ctx, *s)
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// LoadConfig merges an uploaded state into the current state.
// Zones and sources are replaced; streams and presets are additive (deduplicated by ID).
func (c *Controller) LoadConfig(ctx context.Context, incoming models.State) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		// Replace sources and zones
		if incoming.Sources != nil {
			s.Sources = incoming.Sources
		}
		if incoming.Zones != nil {
			s.Zones = incoming.Zones
		}
		if incoming.Groups != nil {
			s.Groups = incoming.Groups
		}

		// Additive merge for streams (dedup by ID)
		if incoming.Streams != nil {
			existingIDs := make(map[int]int) // id → index in s.Streams
			for i, st := range s.Streams {
				existingIDs[st.ID] = i
			}
			for _, st := range incoming.Streams {
				if idx, exists := existingIDs[st.ID]; exists {
					s.Streams[idx] = st // update existing
				} else {
					s.Streams = append(s.Streams, st)
					existingIDs[st.ID] = len(s.Streams) - 1
				}
			}
		}

		// Additive merge for presets (dedup by ID)
		if incoming.Presets != nil {
			existingIDs := make(map[int]int)
			for i, p := range s.Presets {
				existingIDs[p.ID] = i
			}
			for _, p := range incoming.Presets {
				if idx, exists := existingIDs[p.ID]; exists {
					s.Presets[idx] = p
				} else {
					s.Presets = append(s.Presets, p)
					existingIDs[p.ID] = len(s.Presets) - 1
				}
			}
		}

		return c.applyStateToHW(ctx, *s)
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}
