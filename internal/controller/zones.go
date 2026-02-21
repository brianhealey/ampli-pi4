package controller

import (
	"context"
	"fmt"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetZones returns all zones.
func (c *Controller) GetZones() []models.Zone {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]models.Zone, len(c.state.Zones))
	copy(result, c.state.Zones)
	return result
}

// GetZone returns a single zone by ID.
func (c *Controller) GetZone(id int) (*models.Zone, *models.AppError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, z := range c.state.Zones {
		if z.ID == id {
			cp := z
			return &cp, nil
		}
	}
	return nil, models.ErrNotFound("zone not found")
}

// SetZone updates a zone by ID.
func (c *Controller) SetZone(ctx context.Context, id int, upd models.ZoneUpdate) (models.State, *models.AppError) {
	if id < 0 || id >= models.MaxZones {
		return models.State{}, models.ErrBadRequest(fmt.Sprintf("zone id must be 0-%d", models.MaxZones-1))
	}

	state, err := c.apply(func(s *models.State) error {
		z := findZone(s, id)
		if z == nil {
			return models.ErrNotFound("zone not found")
		}
		return applyZoneUpdate(ctx, c, s, z, upd)
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// SetZones performs a bulk zone update.
func (c *Controller) SetZones(ctx context.Context, req models.MultiZoneUpdate) (models.State, *models.AppError) {
	// Validate all zone IDs before applying
	c.mu.RLock()
	for _, id := range req.ZoneIDs {
		if z := findZone(&c.state, id); z == nil {
			c.mu.RUnlock()
			return models.State{}, models.ErrNotFound(fmt.Sprintf("zone %d not found", id))
		}
	}
	c.mu.RUnlock()

	state, err := c.apply(func(s *models.State) error {
		for _, id := range req.ZoneIDs {
			z := findZone(s, id)
			if z == nil {
				return models.ErrNotFound(fmt.Sprintf("zone %d not found", id))
			}
			if err := applyZoneUpdate(ctx, c, s, z, req.Update); err != nil {
				return err
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

// applyZoneUpdate applies a ZoneUpdate to a zone struct and pushes changes to hardware.
func applyZoneUpdate(ctx context.Context, c *Controller, s *models.State, z *models.Zone, upd models.ZoneUpdate) error {
	oldVol := z.Vol
	oldMute := z.Mute
	oldSource := z.SourceID

	if upd.Name != nil {
		z.Name = *upd.Name
	}
	if upd.Disabled != nil {
		z.Disabled = *upd.Disabled
	}
	if upd.SourceID != nil {
		z.SourceID = *upd.SourceID
	}
	if upd.VolMin != nil {
		z.VolMin = *upd.VolMin
	}
	if upd.VolMax != nil {
		z.VolMax = *upd.VolMax
	}

	// Volume updates: vol_f takes precedence, then vol, then vol_delta_f
	if upd.VolF != nil {
		z.Vol = models.VolFToDB(*upd.VolF)
		z.VolF = *upd.VolF
	} else if upd.Vol != nil {
		z.Vol = *upd.Vol
		z.VolF = models.DBToVolF(*upd.Vol)
	} else if upd.VolDeltaF != nil {
		// Apply relative delta: delta maps to a range within [VolMin, VolMax]
		rangeDB := float64(z.VolMax - z.VolMin)
		deltaDB := int(*upd.VolDeltaF * rangeDB)
		z.Vol = z.Vol + deltaDB
		z.VolF = models.DBToVolF(z.Vol)
	}

	// Clamp vol to zone limits
	z.Vol = models.ClampVol(z.Vol, z.VolMin, z.VolMax)
	z.VolF = models.DBToVolF(z.Vol)

	if upd.Mute != nil {
		z.Mute = *upd.Mute
	}

	// Push to hardware
	unit := z.ID / 6
	localZone := z.ID % 6

	if z.SourceID != oldSource {
		// Rebuild zone sources for this unit
		if err := pushZoneSources(ctx, c, s, unit); err != nil {
			return err
		}
	}

	if z.Vol != oldVol {
		if err := c.hw.SetZoneVol(ctx, unit, localZone, z.Vol); err != nil {
			return err
		}
	}

	if z.Mute != oldMute {
		if err := pushZoneMutes(ctx, c, s, unit); err != nil {
			return err
		}
	}

	// Update group aggregates
	updateGroupAggregates(s)

	return nil
}

// pushZoneSources writes zone source assignments for a unit to hardware.
func pushZoneSources(ctx context.Context, c *Controller, s *models.State, unit int) error {
	baseZone := unit * 6
	var sources [6]int
	for i := 0; i < 6; i++ {
		zoneIdx := baseZone + i
		if z := findZone(s, zoneIdx); z != nil {
			src := z.SourceID
			if src < 0 || src > 3 {
				src = 0
			}
			sources[i] = src
		}
	}
	return c.hw.SetZoneSources(ctx, unit, sources)
}

// pushZoneMutes writes zone mute states for a unit to hardware.
func pushZoneMutes(ctx context.Context, c *Controller, s *models.State, unit int) error {
	baseZone := unit * 6
	var mutes [6]bool
	for i := 0; i < 6; i++ {
		zoneIdx := baseZone + i
		if z := findZone(s, zoneIdx); z != nil {
			mutes[i] = z.Mute
		} else {
			mutes[i] = true
		}
	}
	return c.hw.SetZoneMutes(ctx, unit, mutes)
}
