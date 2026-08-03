package models

import (
	"fmt"
	"sort"

	"github.com/micro-nova/amplipi-go/internal/hardware"
)

// ReconcileZones adds a zone entry for every zone the hardware profile reports
// that state does not already have, and returns how many it added.
//
// This exists because the zone list is otherwise fixed when the config is first
// written. A config created with one unit attached never grows when an expander
// is added later: the hardware layer reports the extra zones, but they never
// reach state, so they are invisible to the API and UI — and applyStateToHW
// leaves their amps disabled precisely because they are missing.
//
// New zones arrive disabled and muted. They are wired to rooms that may not
// exist yet, so they must not start driving speakers the moment hardware is
// detected.
//
// Existing zones are never modified, and zones whose hardware is absent are
// never removed — unplugging an expander should not destroy the names and
// volumes configured for it.
func ReconcileZones(state *State, p *hardware.HardwareProfile) int {
	if state == nil || p == nil {
		return 0 // no profile means we could not read the hardware; change nothing
	}

	have := make(map[int]bool, len(state.Zones))
	for _, z := range state.Zones {
		have[z.ID] = true
	}

	added := 0
	for _, unit := range p.Units {
		if unit.Board.UnitType == hardware.UnitTypeStreamer {
			continue // streamer units have no amplified zones
		}
		for i := 0; i < unit.ZoneCount; i++ {
			id := unit.ZoneBase + i
			if have[id] || id < 0 || id >= MaxZones {
				continue
			}
			state.Zones = append(state.Zones, Zone{
				ID:       id,
				Name:     fmt.Sprintf("Zone %d", id+1),
				SourceID: 0,
				Mute:     true,
				Vol:      MinVolDB,
				VolF:     0.0,
				VolMin:   MinVolDB,
				VolMax:   MaxVolDB,
				Disabled: true,
			})
			have[id] = true
			added++
		}
	}

	if added > 0 {
		sort.Slice(state.Zones, func(i, j int) bool { return state.Zones[i].ID < state.Zones[j].ID })
	}
	return added
}
