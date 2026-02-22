package config

import (
	"fmt"
	"log/slog"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// migrateState fills in default values for fields that may be missing
// in older config files or Python-format configs.
func migrateState(state *models.State) {
	def := models.DefaultState()

	// Ensure sources slice has at least 4 entries
	for len(state.Sources) < 4 {
		idx := len(state.Sources)
		if idx < len(def.Sources) {
			state.Sources = append(state.Sources, def.Sources[idx])
		} else {
			break
		}
	}

	// Validate and fix source IDs
	for i := range state.Sources {
		if state.Sources[i].ID < 0 || state.Sources[i].ID > 3 {
			slog.Warn("config: invalid source ID, fixing", "id", state.Sources[i].ID, "index", i)
			state.Sources[i].ID = i
		}
	}

	// Validate and fix zone fields
	for i := range state.Zones {
		z := &state.Zones[i]
		if z.ID < 0 || z.ID > models.MaxZones-1 {
			slog.Warn("config: invalid zone ID, fixing", "id", z.ID, "index", i)
			z.ID = i
		}
		// Apply defaults for unset volume limits
		if z.VolMin == 0 && z.VolMax == 0 {
			z.VolMin = models.MinVolDB
			z.VolMax = models.MaxVolDB
		}
		// Clamp vol to configured limits
		if z.Vol < z.VolMin {
			z.Vol = z.VolMin
		}
		if z.Vol > z.VolMax {
			z.Vol = z.VolMax
		}
		// Sync vol_f from vol if not set
		if z.VolF == 0 && z.Vol != z.VolMin {
			z.VolF = models.DBToVolF(z.Vol)
		}
	}

	// Validate group IDs
	for i := range state.Groups {
		g := &state.Groups[i]
		if g.ID < 0 {
			slog.Warn("config: invalid group ID, fixing", "id", g.ID, "index", i)
			g.ID = 100 + i
		}
		if g.ZoneIDs == nil {
			g.ZoneIDs = []int{}
		}
	}

	// Validate stream IDs
	for i := range state.Streams {
		if state.Streams[i].ID < 0 {
			slog.Warn("config: invalid stream ID, fixing", "id", state.Streams[i].ID, "index", i)
			state.Streams[i].ID = 1000 + i
		}
	}

	// Ensure default RCA and Aux streams exist (needed for physical RCA inputs)
	ensureDefaultStreams(state)

	// Validate preset IDs
	for i := range state.Presets {
		if state.Presets[i].ID < 0 {
			slog.Warn("config: invalid preset ID, fixing", "id", state.Presets[i].ID, "index", i)
			state.Presets[i].ID = i
		}
	}

	// Ensure slices are not nil
	if state.Groups == nil {
		state.Groups = []models.Group{}
	}
	if state.Streams == nil {
		state.Streams = []models.Stream{}
	}
	if state.Presets == nil {
		state.Presets = []models.Preset{}
	}
}

// ensureDefaultStreams adds missing default RCA and Aux streams to the state.
// These streams represent physical hardware inputs and should always be present.
func ensureDefaultStreams(state *models.State) {
	// Check which default streams are missing
	hasAux := false
	rcaPresent := make(map[int]bool)

	for _, s := range state.Streams {
		if s.ID == models.AuxStreamID && s.Type == models.StreamTypeAux {
			hasAux = true
		}
		if s.Type == models.StreamTypeRCA {
			rcaPresent[s.ID] = true
		}
	}

	// Add missing default streams
	f := false

	if !hasAux {
		slog.Info("config: adding missing Aux stream")
		state.Streams = append(state.Streams, models.Stream{
			ID:        models.AuxStreamID,
			Name:      "Aux",
			Type:      models.StreamTypeAux,
			Disabled:  &f,
			Browsable: &f,
		})
	}

	// Add missing RCA streams (Input 1-4)
	rcaIDs := []int{models.RCAStream0, models.RCAStream1, models.RCAStream2, models.RCAStream3}
	for i, rcaID := range rcaIDs {
		if !rcaPresent[rcaID] {
			name := fmt.Sprintf("Input %d", i+1)
			slog.Info("config: adding missing RCA stream", "id", rcaID, "name", name)
			state.Streams = append(state.Streams, models.Stream{
				ID:        rcaID,
				Name:      name,
				Type:      models.StreamTypeRCA,
				Disabled:  &f,
				Browsable: &f,
			})
		}
	}
}
