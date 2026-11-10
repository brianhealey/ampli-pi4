package controller

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetSources returns all sources.
func (c *Controller) GetSources() []models.Source {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.withEQStatus(c.state).Sources
}

// GetSource returns a single source by ID.
func (c *Controller) GetSource(id int) (*models.Source, *models.AppError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, s := range c.withEQStatus(c.state).Sources {
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

	if appErr := validateEQ(upd.EQ); appErr != nil {
		return models.State{}, appErr
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
		if upd.EQ != nil {
			src.EQ = upd.EQ.Clone()
		}
		if upd.Input != nil {
			if appErr := checkSourceDAC(c.profile, s, id, *upd.Input); appErr != nil {
				return appErr
			}
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

// digitalStreamFor returns the stream an input selects when that stream needs a
// DAC (anything other than an RCA/Aux passthrough). It returns nil for analog
// inputs, the empty input, and references to streams that do not exist.
func digitalStreamFor(input string, state *models.State) *models.Stream {
	if !strings.HasPrefix(input, "stream=") || isAnalogInput(input, state) {
		return nil
	}
	id, err := strconv.Atoi(strings.TrimPrefix(input, "stream="))
	if err != nil {
		return nil
	}
	return findStream(state, id)
}

// checkSourceDAC rejects a digital stream on a source that no DAC feeds.
// On units without the USB DAC only source 0 (HiFiBerry) has one; without this
// check the stream would play into nothing while reporting "playing".
func checkSourceDAC(profile *hardware.HardwareProfile, state *models.State, srcID int, input string) *models.AppError {
	if profile.SourceHasDAC(srcID) {
		return nil
	}
	if digitalStreamFor(input, state) == nil {
		return nil
	}
	return models.ErrBadRequest(noDACMessage(profile, srcID))
}

// noDACMessage explains which sources can carry digital streams on this unit.
func noDACMessage(profile *hardware.HardwareProfile, srcID int) string {
	digital := profile.DigitalSources()
	var where string
	switch len(digital) {
	case 0:
		where = "this unit has no DAC, so digital streams cannot play"
	case 1:
		where = fmt.Sprintf("digital streams can only play on source %d", digital[0])
	default:
		ids := make([]string, len(digital))
		for i, d := range digital {
			ids[i] = strconv.Itoa(d)
		}
		where = "digital streams can only play on sources " + strings.Join(ids, ", ")
	}
	return fmt.Sprintf("source %d has no digital output on this unit; %s", srcID, where)
}

// enforceSourceDACs marks each source with whether a DAC feeds it and clears
// any digital stream sitting on a source without one, logging one warning per
// cleared source. Used on every path that replaces sources wholesale (startup
// load, config restore, factory reset) so state never claims a stream is
// playing somewhere it cannot be heard. Analog inputs are never touched.
// Returns the number of inputs cleared.
func enforceSourceDACs(state *models.State, profile *hardware.HardwareProfile) int {
	cleared := 0
	for i := range state.Sources {
		src := &state.Sources[i]
		if profile.SourceHasDAC(src.ID) {
			src.HasDAC = nil
			continue
		}
		noDAC := false
		src.HasDAC = &noDAC
		if st := digitalStreamFor(src.Input, state); st != nil {
			slog.Warn("controller: clearing digital stream from source without a DAC",
				"source", src.ID, "stream_id", st.ID, "stream", st.Name, "type", st.Type,
				"digital_sources", profile.DigitalSources())
			src.Input = ""
			cleared++
		}
	}
	return cleared
}

// sourceAnalogFlags derives the per-source analog/digital flags from state.
// Index is the source ID; true means analog (RCA/Aux passthrough).
// This is the single source of truth for the RegSrcAD register contents — both
// startup and runtime input changes must derive the flags the same way.
func sourceAnalogFlags(state *models.State) [4]bool {
	var analog [4]bool
	for i := range state.Sources {
		src := &state.Sources[i]
		if src.ID >= 0 && src.ID <= 3 {
			analog[src.ID] = isAnalogInput(src.Input, state)
		}
	}
	return analog
}

// updateSourceTypeHW updates the hardware source type (analog/digital) registers.
func (c *Controller) updateSourceTypeHW(ctx context.Context, state *models.State, _ int) error {
	analog := sourceAnalogFlags(state)
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
