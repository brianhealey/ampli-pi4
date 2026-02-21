// Package models defines the data structures for the AmpliPi system.
// JSON field names match the Python implementation exactly for wire compatibility.
package models

// Source represents one of the 4 audio inputs. Each can have a stream connected.
type Source struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"` // "" | "local" | "stream=<id>" | "RCA" | "aux"
}

// Zone represents one of up to 36 amplified outputs.
type Zone struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	SourceID int     `json:"source_id"`
	Mute     bool    `json:"mute"`
	Vol      int     `json:"vol"`     // dB attenuation, range [-80, 0]
	VolF     float64 `json:"vol_f"`   // Volume as float [0.0, 1.0]
	VolMin   int     `json:"vol_min"` // default -80
	VolMax   int     `json:"vol_max"` // default 0
	Disabled bool    `json:"disabled"` // hardware not present
}

// Group is a named collection of zones controlled together.
type Group struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	ZoneIDs  []int   `json:"zones"`
	SourceID *int    `json:"source_id,omitempty"` // nullable
	Vol      *int    `json:"vol_delta,omitempty"` // nullable — average vol delta from zone base
	VolF     *float64 `json:"vol_f,omitempty"`    // nullable — average vol as float
	Mute     *bool   `json:"mute,omitempty"`      // nullable
}

// StreamInfo is the runtime status of a stream (what it's playing, album art URL, etc.)
type StreamInfo struct {
	Name     string `json:"name"`
	State    string `json:"state"` // "playing" | "paused" | "stopped" | "disconnected" | "loading"
	Track    string `json:"track,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Station  string `json:"station,omitempty"`
	ImageURL string `json:"img_url,omitempty"`
	Rating   *int   `json:"rating,omitempty"`
}

// Stream is a configured audio source (Pandora, AirPlay, etc.)
type Stream struct {
	ID     int                    `json:"id"`
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Info   StreamInfo             `json:"info,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
	// Flat stream-type-specific fields for JSON compatibility with Python
	Disabled  *bool `json:"disabled,omitempty"`
	Browsable *bool `json:"browsable,omitempty"`
}

// Preset is a saved system state snapshot.
type Preset struct {
	ID       int          `json:"id"`
	Name     string       `json:"name"`
	State    *PresetState `json:"state,omitempty"`
	Commands []Command    `json:"commands,omitempty"`
}

// PresetState is the partial state to be applied when loading a preset.
type PresetState struct {
	Sources []SourceUpdate `json:"sources,omitempty"`
	Zones   []ZoneUpdate   `json:"zones,omitempty"`
	Groups  []GroupUpdate  `json:"groups,omitempty"`
}

// Command is an action to execute as part of loading a preset.
type Command struct {
	Endpoint string                 `json:"endpoint"`
	Method   string                 `json:"method"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// Info is the system information response.
type Info struct {
	Version  string `json:"version"`
	UnitID   int    `json:"unit_id,omitempty"`
	IsUpdate bool   `json:"is_update,omitempty"`
	Offline  bool   `json:"offline"`
}

// State is the complete system state returned by GET /api.
// Corresponds to Python's models.Status.
type State struct {
	Sources []Source `json:"sources"`
	Zones   []Zone   `json:"zones"`
	Groups  []Group  `json:"groups"`
	Streams []Stream `json:"streams"`
	Presets []Preset `json:"presets"`
	Info    Info     `json:"info"`
}

// deepCopy returns a deep copy of the state.
func (s State) DeepCopy() State {
	next := State{
		Info: s.Info,
	}

	// Copy sources
	next.Sources = make([]Source, len(s.Sources))
	copy(next.Sources, s.Sources)

	// Copy zones
	next.Zones = make([]Zone, len(s.Zones))
	copy(next.Zones, s.Zones)

	// Copy groups (need deep copy of ZoneIDs slice)
	next.Groups = make([]Group, len(s.Groups))
	for i, g := range s.Groups {
		ng := g
		if g.ZoneIDs != nil {
			ng.ZoneIDs = make([]int, len(g.ZoneIDs))
			copy(ng.ZoneIDs, g.ZoneIDs)
		}
		if g.SourceID != nil {
			v := *g.SourceID
			ng.SourceID = &v
		}
		if g.Vol != nil {
			v := *g.Vol
			ng.Vol = &v
		}
		if g.VolF != nil {
			v := *g.VolF
			ng.VolF = &v
		}
		if g.Mute != nil {
			v := *g.Mute
			ng.Mute = &v
		}
		next.Groups[i] = ng
	}

	// Copy streams (Config map needs deep copy)
	next.Streams = make([]Stream, len(s.Streams))
	for i, st := range s.Streams {
		ns := st
		if st.Config != nil {
			ns.Config = make(map[string]interface{}, len(st.Config))
			for k, v := range st.Config {
				ns.Config[k] = v
			}
		}
		if st.Disabled != nil {
			v := *st.Disabled
			ns.Disabled = &v
		}
		if st.Browsable != nil {
			v := *st.Browsable
			ns.Browsable = &v
		}
		next.Streams[i] = ns
	}

	// Copy presets (State and Commands need deep copy)
	next.Presets = make([]Preset, len(s.Presets))
	for i, p := range s.Presets {
		np := Preset{ID: p.ID, Name: p.Name}
		if p.State != nil {
			ps := *p.State
			np.State = &ps
		}
		if p.Commands != nil {
			np.Commands = make([]Command, len(p.Commands))
			copy(np.Commands, p.Commands)
		}
		next.Presets[i] = np
	}

	return next
}

// Constants for special source IDs.
const (
	SourceDisconnected = -1 // No source connection
	ZoneOff            = -2 // Zone is off (for HA integration)
	MaxSources         = 4
	MaxZones           = 36

	MinVolDB = -80
	MaxVolDB = 0
)
