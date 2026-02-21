package models

import "fmt"

// DefaultState returns a minimal default system state — 4 sources, 6 zones, no groups/streams/presets.
// This is the minimal state used when no config file is found.
// Based on Python's defaults.py DEFAULT_CONFIG.
func DefaultState() State {
	sources := make([]Source, 4)
	for i := range sources {
		sources[i] = Source{
			ID:    i,
			Name:  fmt.Sprintf("Output %d", i+1),
			Input: "",
		}
	}

	zones := make([]Zone, 6)
	for i := range zones {
		zones[i] = Zone{
			ID:       i,
			Name:     fmt.Sprintf("Zone %d", i+1),
			SourceID: 0,
			Mute:     true,
			Vol:      MinVolDB,
			VolF:     0.0,
			VolMin:   MinVolDB,
			VolMax:   MaxVolDB,
			Disabled: false,
		}
	}

	// Mute All preset (id 10000) matching Python defaults.MUTE_ALL_ID
	muteAllZones := make([]ZoneUpdate, 6)
	for i := range muteAllZones {
		id := i
		mute := true
		muteAllZones[i] = ZoneUpdate{ID: &id, Mute: &mute}
	}
	presets := []Preset{
		{
			ID:   MuteAllPresetID,
			Name: "Mute All",
			State: &PresetState{
				Zones: muteAllZones,
			},
		},
	}

	// Default streams: Aux + 4 RCA inputs
	f := false
	streams := []Stream{
		{ID: AuxStreamID, Name: "Aux", Type: StreamTypeAux, Disabled: &f, Browsable: &f},
		{ID: RCAStream0, Name: "Input 1", Type: StreamTypeRCA, Disabled: &f, Browsable: &f},
		{ID: RCAStream1, Name: "Input 2", Type: StreamTypeRCA, Disabled: &f, Browsable: &f},
		{ID: RCAStream2, Name: "Input 3", Type: StreamTypeRCA, Disabled: &f, Browsable: &f},
		{ID: RCAStream3, Name: "Input 4", Type: StreamTypeRCA, Disabled: &f, Browsable: &f},
	}

	return State{
		Sources: sources,
		Zones:   zones,
		Groups:  []Group{},
		Streams: streams,
		Presets: presets,
		Info: Info{
			Version: "0.0.1",
			Offline: false,
		},
	}
}

// Preset IDs from Python defaults.
const (
	MuteAllPresetID  = 10000
	LastPresetID     = 9999
)

// VolFToDB converts a float volume [0.0, 1.0] to dB [-80, 0].
func VolFToDB(f float64) int {
	if f < 0.0 {
		f = 0.0
	}
	if f > 1.0 {
		f = 1.0
	}
	return int(f*float64(MaxVolDB-MinVolDB)) + MinVolDB
}

// DBToVolF converts a dB volume [-80, 0] to float [0.0, 1.0].
func DBToVolF(db int) float64 {
	if db < MinVolDB {
		db = MinVolDB
	}
	if db > MaxVolDB {
		db = MaxVolDB
	}
	return float64(db-MinVolDB) / float64(MaxVolDB-MinVolDB)
}

// ClampVol clamps a volume value to the zone's configured min/max.
func ClampVol(vol, volMin, volMax int) int {
	if vol < volMin {
		return volMin
	}
	if vol > volMax {
		return volMax
	}
	return vol
}
