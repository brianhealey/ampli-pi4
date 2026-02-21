package controller

import (
	"context"
	"fmt"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetGroups returns all groups.
func (c *Controller) GetGroups() []models.Group {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]models.Group, len(c.state.Groups))
	copy(result, c.state.Groups)
	return result
}

// GetGroup returns a single group by ID.
func (c *Controller) GetGroup(id int) (*models.Group, *models.AppError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	g := findGroup(&c.state, id)
	if g == nil {
		return nil, models.ErrNotFound("group not found")
	}
	cp := *g
	return &cp, nil
}

// CreateGroup creates a new group and returns the updated state.
func (c *Controller) CreateGroup(ctx context.Context, req models.GroupUpdate) (models.State, *models.AppError) {
	if req.Name == nil || *req.Name == "" {
		return models.State{}, models.ErrBadRequest("group name is required")
	}

	state, err := c.apply(func(s *models.State) error {
		g := models.Group{
			ID:      nextGroupID(s),
			Name:    *req.Name,
			ZoneIDs: req.ZoneIDs,
		}
		if req.SourceID != nil {
			v := *req.SourceID
			g.SourceID = &v
		}
		if req.Mute != nil {
			v := *req.Mute
			g.Mute = &v
		}
		s.Groups = append(s.Groups, g)
		updateGroupAggregates(s)
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

// SetGroup updates a group by ID.
func (c *Controller) SetGroup(ctx context.Context, id int, upd models.GroupUpdate) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		g := findGroup(s, id)
		if g == nil {
			return models.ErrNotFound("group not found")
		}

		if upd.Name != nil {
			g.Name = *upd.Name
		}
		if upd.ZoneIDs != nil {
			g.ZoneIDs = upd.ZoneIDs
		}
		if upd.SourceID != nil {
			v := *upd.SourceID
			g.SourceID = &v
			// Apply source to all member zones
			for _, zid := range g.ZoneIDs {
				z := findZone(s, zid)
				if z == nil {
					continue
				}
				src := *upd.SourceID
				zupd := models.ZoneUpdate{SourceID: &src}
				if err := applyZoneUpdate(ctx, c, s, z, zupd); err != nil {
					return err
				}
			}
		}

		// Volume delta: apply to each member zone
		if upd.Vol != nil {
			for _, zid := range g.ZoneIDs {
				z := findZone(s, zid)
				if z == nil {
					continue
				}
				newVol := z.Vol + *upd.Vol
				clamped := models.ClampVol(newVol, z.VolMin, z.VolMax)
				zupd := models.ZoneUpdate{Vol: &clamped}
				if err := applyZoneUpdate(ctx, c, s, z, zupd); err != nil {
					return err
				}
			}
		} else if upd.VolF != nil {
			// VolF sets absolute float volume on all zones
			for _, zid := range g.ZoneIDs {
				z := findZone(s, zid)
				if z == nil {
					continue
				}
				vf := *upd.VolF
				zupd := models.ZoneUpdate{VolF: &vf}
				if err := applyZoneUpdate(ctx, c, s, z, zupd); err != nil {
					return err
				}
			}
		}

		// Mute: apply to all member zones
		if upd.Mute != nil {
			for _, zid := range g.ZoneIDs {
				z := findZone(s, zid)
				if z == nil {
					continue
				}
				m := *upd.Mute
				zupd := models.ZoneUpdate{Mute: &m}
				if err := applyZoneUpdate(ctx, c, s, z, zupd); err != nil {
					return err
				}
			}
		}

		updateGroupAggregates(s)
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

// DeleteGroup removes a group by ID.
func (c *Controller) DeleteGroup(_ context.Context, id int) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		for i, g := range s.Groups {
			if g.ID == id {
				s.Groups = append(s.Groups[:i], s.Groups[i+1:]...)
				return nil
			}
		}
		return models.ErrNotFound(fmt.Sprintf("group %d not found", id))
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// updateGroupAggregates recomputes aggregate vol_delta, mute, and source_id for all groups.
func updateGroupAggregates(s *models.State) {
	for gi := range s.Groups {
		g := &s.Groups[gi]
		if len(g.ZoneIDs) == 0 {
			continue
		}

		allMuted := true
		anyMuted := false
		totalVol := 0
		validZones := 0
		var unanimousSource *int

		for _, zid := range g.ZoneIDs {
			z := findZone(s, zid)
			if z == nil {
				continue
			}
			totalVol += z.Vol
			validZones++
			if z.Mute {
				anyMuted = true
			} else {
				allMuted = false
			}

			// Track unanimous source
			if unanimousSource == nil {
				src := z.SourceID
				unanimousSource = &src
			} else if *unanimousSource != z.SourceID {
				unanimousSource = nil // not unanimous
			}
		}

		if validZones > 0 {
			avgVol := totalVol / validZones
			g.Vol = &avgVol
			avgVolF := models.DBToVolF(avgVol)
			g.VolF = &avgVolF
		}

		mute := allMuted
		if !allMuted && !anyMuted {
			mute = false
		}
		g.Mute = &mute
		g.SourceID = unanimousSource
	}
}
