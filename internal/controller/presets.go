package controller

import (
	"context"
	"fmt"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetPresets returns all presets.
func (c *Controller) GetPresets() []models.Preset {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]models.Preset, len(c.state.Presets))
	copy(result, c.state.Presets)
	return result
}

// GetPreset returns a single preset by ID.
func (c *Controller) GetPreset(id int) (*models.Preset, *models.AppError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p := findPreset(&c.state, id)
	if p == nil {
		return nil, models.ErrNotFound("preset not found")
	}
	cp := *p
	return &cp, nil
}

// CreatePreset creates a new preset.
func (c *Controller) CreatePreset(_ context.Context, req models.PresetCreate) (models.State, *models.AppError) {
	if req.Name == "" {
		return models.State{}, models.ErrBadRequest("preset name is required")
	}

	state, err := c.apply(func(s *models.State) error {
		p := models.Preset{
			ID:       nextPresetID(s),
			Name:     req.Name,
			State:    req.State,
			Commands: req.Commands,
		}
		s.Presets = append(s.Presets, p)
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

// SetPreset updates a preset by ID.
func (c *Controller) SetPreset(_ context.Context, id int, upd models.PresetUpdate) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		p := findPreset(s, id)
		if p == nil {
			return models.ErrNotFound(fmt.Sprintf("preset %d not found", id))
		}
		if upd.Name != nil {
			p.Name = *upd.Name
		}
		if upd.State != nil {
			p.State = upd.State
		}
		if upd.Commands != nil {
			p.Commands = upd.Commands
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

// DeletePreset removes a preset by ID.
func (c *Controller) DeletePreset(_ context.Context, id int) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		for i, p := range s.Presets {
			if p.ID == id {
				s.Presets = append(s.Presets[:i], s.Presets[i+1:]...)
				return nil
			}
		}
		return models.ErrNotFound(fmt.Sprintf("preset %d not found", id))
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// LoadPreset applies a preset's state and commands to the system.
func (c *Controller) LoadPreset(ctx context.Context, id int) (models.State, *models.AppError) {
	// Get the preset to load
	c.mu.RLock()
	p := findPreset(&c.state, id)
	if p == nil {
		c.mu.RUnlock()
		return models.State{}, models.ErrNotFound(fmt.Sprintf("preset %d not found", id))
	}
	preset := *p
	c.mu.RUnlock()

	state, err := c.apply(func(s *models.State) error {
		if preset.State == nil {
			return nil
		}
		ps := preset.State

		// Apply source updates
		for _, upd := range ps.Sources {
			if upd.ID == nil {
				continue
			}
			src := findSourceInState(s, *upd.ID)
			if src == nil {
				continue
			}
			if upd.Name != nil {
				src.Name = *upd.Name
			}
			if upd.Input != nil {
				src.Input = *upd.Input
			}
		}

		// Apply zone updates
		for _, upd := range ps.Zones {
			if upd.ID == nil {
				continue
			}
			z := findZone(s, *upd.ID)
			if z == nil {
				continue
			}
			if err := applyZoneUpdate(ctx, c, s, z, upd); err != nil {
				return err
			}
		}

		// Apply group updates
		for _, upd := range ps.Groups {
			if upd.ID == nil {
				continue
			}
			g := findGroup(s, *upd.ID)
			if g == nil {
				continue
			}
			if upd.Name != nil {
				g.Name = *upd.Name
			}
			if upd.SourceID != nil {
				v := *upd.SourceID
				g.SourceID = &v
			}
			if upd.Mute != nil {
				v := *upd.Mute
				g.Mute = &v
			}
		}

		// TODO Phase 3: execute preset Commands via stream subsystem
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

func findSourceInState(s *models.State, id int) *models.Source {
	for i := range s.Sources {
		if s.Sources[i].ID == id {
			return &s.Sources[i]
		}
	}
	return nil
}
