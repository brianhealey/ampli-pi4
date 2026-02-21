package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetSources returns all sources.
func (c *Controller) GetSources() []models.Source {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]models.Source, len(c.state.Sources))
	copy(result, c.state.Sources)
	return result
}

// GetSource returns a single source by ID.
func (c *Controller) GetSource(id int) (*models.Source, *models.AppError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, s := range c.state.Sources {
		if s.ID == id {
			cp := s
			return &cp, nil
		}
	}
	return nil, models.ErrNotFound("source not found")
}

// validateSourceInput checks hardware capability constraints for a source input change.
// Returns a non-nil error if the profile prohibits the requested input on this hardware.
// Returns nil if profile is nil (no restrictions — used in tests/mock mode).
func (c *Controller) validateSourceInput(input string) *models.AppError {
	if c.profile == nil {
		return nil
	}
	if c.profile.TotalSources == 0 {
		return models.ErrBadRequest("this unit has no audio sources")
	}
	// Read the current state to resolve stream types
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	if isAnalogInput(input, &state) && !c.profile.HasMainUnit() {
		return models.ErrBadRequest(fmt.Sprintf("analog input not supported on %s unit", c.profile.PrimaryUnitType()))
	}
	return nil
}

// SetSource updates a source by ID and returns the new state.
func (c *Controller) SetSource(ctx context.Context, id int, upd models.SourceUpdate) (models.State, *models.AppError) {
	if id < 0 || id > 3 {
		return models.State{}, models.ErrBadRequest("source id must be 0-3")
	}

	// Validate hardware capability before applying
	if upd.Input != nil {
		if appErr := c.validateSourceInput(*upd.Input); appErr != nil {
			return models.State{}, appErr
		}
	}

	state, err := c.apply(func(s *models.State) error {
		var src *models.Source
		for i := range s.Sources {
			if s.Sources[i].ID == id {
				src = &s.Sources[i]
				break
			}
		}
		if src == nil {
			return models.ErrNotFound("source not found")
		}

		if upd.Name != nil {
			src.Name = *upd.Name
		}
		if upd.Input != nil {
			oldInput := src.Input
			src.Input = *upd.Input
			if oldInput != *upd.Input {
				// Update hardware source type (analog/digital)
				_ = c.updateSourceTypeHW(ctx, s, id)
			}
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

// updateSourceTypeHW updates the hardware source type (analog/digital) registers.
func (c *Controller) updateSourceTypeHW(ctx context.Context, state *models.State, _ int) error {
	var analog [4]bool
	for i := range state.Sources {
		src := &state.Sources[i]
		if src.ID >= 0 && src.ID <= 3 {
			analog[src.ID] = isAnalogInput(src.Input, state)
		}
	}
	for _, unit := range c.hw.Units() {
		if err := c.hw.SetSourceTypes(ctx, unit, analog); err != nil {
			return err
		}
	}
	return nil
}

// isAnalogInput returns true if the input string corresponds to an analog source.
// Analog sources: "local", or stream=<id> where the stream is RCA or Aux type.
func isAnalogInput(input string, state *models.State) bool {
	if input == "local" {
		return true
	}
	if strings.HasPrefix(input, "stream=") {
		idStr := strings.TrimPrefix(input, "stream=")
		streamID, err := strconv.Atoi(idStr)
		if err != nil {
			return false
		}
		// RCA and Aux stream IDs are always analog
		if streamID == models.AuxStreamID ||
			(streamID >= models.RCAStream0 && streamID <= models.RCAStream3) {
			return true
		}
		// Check stream type in state
		for _, s := range state.Streams {
			if s.ID == streamID {
				return s.Type == models.StreamTypeRCA || s.Type == models.StreamTypeAux
			}
		}
	}
	return false
}
