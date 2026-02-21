package controller

import (
	"context"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetInfo returns system information.
func (c *Controller) GetInfo() models.Info {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state.Info
}

// FactoryReset resets the system to default state and pushes it to hardware.
func (c *Controller) FactoryReset(ctx context.Context) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		// Preserve the current version info
		info := s.Info
		*s = models.DefaultState()
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
