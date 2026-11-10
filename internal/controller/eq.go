package controller

import (
	"fmt"
	"log/slog"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// Per-source EQ. The setting (models.SourceEQ) is persisted on the source
// through apply() like any other field; whether it is actually applied
// (models.EQStatus) is derived when state is read, from the hardware profile,
// the source's input and the stream manager's runtime bridge state. The
// status is never stored in c.state, so it never reaches house.json.
//
// An EQ set while CamillaDSP is missing is stored and reported "unavailable"
// rather than rejected: unlike a stream type with no binary (CreateStream's
// 400), an EQ is a setting on a source that already exists and degrades to
// exactly today's audio, and it must survive a config restore from another
// unit or a later CamillaDSP install. The same goes for sources without a
// DAC ("no_dac") and analog inputs ("analog").

// validateEQ turns an EQ validation error into a 400.
func validateEQ(eq *models.SourceEQ) *models.AppError {
	if err := eq.Validate(); err != nil {
		return models.ErrBadRequest(err.Error())
	}
	return nil
}

// eqAvailable reports whether CamillaDSP is installed on this unit.
func (c *Controller) eqAvailable() bool {
	return c.profile.EQAvailable()
}

// eqStatusFor derives a source's EQ status. Nil when the source has no EQ
// configured, so sources without EQ look exactly as they did before.
func (c *Controller) eqStatusFor(src *models.Source, s *models.State) *models.EQStatus {
	if src.EQ == nil {
		return nil
	}
	st := func(state, reason string) *models.EQStatus {
		return &models.EQStatus{State: state, Reason: reason}
	}
	switch {
	case !src.EQ.Enabled:
		return st(models.EQStateOff, "")
	case !c.eqAvailable():
		return st(models.EQStateUnavailable, "CamillaDSP is not installed on this unit")
	case !c.profile.SourceHasDAC(src.ID):
		return st(models.EQStateNoDAC, fmt.Sprintf("source %d has no DAC, so no digital stream plays here", src.ID))
	case src.Input == "":
		return st(models.EQStateIdle, "no input selected")
	case isAnalogInput(src.Input, s):
		return st(models.EQStateAnalog, "RCA and aux inputs go straight to the analog preamp; EQ applies to digital streams only")
	case digitalStreamFor(src.Input, s) == nil:
		return st(models.EQStateIdle, "the selected stream does not exist")
	case c.streams == nil:
		return st(models.EQStateIdle, "the stream subsystem is not running")
	}
	if rt, ok := c.streams.EQStatus(src.ID); ok {
		return &rt
	}
	return st(models.EQStateStarting, "waiting for the stream to connect")
}

// withEQStatus returns s with each source's EQ status and the unit's EQ
// availability filled in. The Sources slice is copied, so s may share it with
// c.state. Safe to call with c.mu held: the stream manager's EQStatus only
// takes its own status lock.
func (c *Controller) withEQStatus(s models.State) models.State {
	srcs := make([]models.Source, len(s.Sources))
	copy(srcs, s.Sources)
	for i := range srcs {
		srcs[i].EQStatus = c.eqStatusFor(&srcs[i], &s)
	}
	s.Sources = srcs
	s.Info.EQAvailable = c.eqAvailable()
	return s
}

// sanitizeSourceEQs clears status fields that came in from storage or an
// uploaded config, and drops EQ settings that fail validation (hand-edited or
// from a newer version) with a warning, so the bridge never gets values the
// API would have refused. Returns the number of EQs dropped.
func sanitizeSourceEQs(state *models.State) int {
	dropped := 0
	for i := range state.Sources {
		src := &state.Sources[i]
		src.EQStatus = nil
		if err := src.EQ.Validate(); err != nil {
			slog.Warn("controller: dropping invalid source EQ", "source", src.ID, "err", err)
			src.EQ = nil
			dropped++
		}
	}
	return dropped
}

// publishState sends the current state, with fresh EQ status, to event
// subscribers. Used when only runtime EQ status changed.
func (c *Controller) publishState() {
	c.bus.Publish(c.State())
}
