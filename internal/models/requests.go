package models

// SourceUpdate is the PATCH body for updating a source.
type SourceUpdate struct {
	ID    *int    `json:"id,omitempty"`
	Name  *string `json:"name,omitempty"`
	Input *string `json:"input,omitempty"`
}

// ZoneUpdate is the PATCH body for updating a zone.
type ZoneUpdate struct {
	ID       *int     `json:"id,omitempty"`
	Name     *string  `json:"name,omitempty"`
	SourceID *int     `json:"source_id,omitempty"`
	Mute     *bool    `json:"mute,omitempty"`
	Vol      *int     `json:"vol,omitempty"`
	VolF     *float64 `json:"vol_f,omitempty"`
	VolDeltaF *float64 `json:"vol_delta_f,omitempty"`
	VolMin   *int     `json:"vol_min,omitempty"`
	VolMax   *int     `json:"vol_max,omitempty"`
	Disabled *bool    `json:"disabled,omitempty"`
}

// MultiZoneUpdate is the PATCH body for bulk zone updates.
type MultiZoneUpdate struct {
	ZoneIDs []int      `json:"zones"`
	Update  ZoneUpdate `json:"update"`
}

// GroupUpdate is the PATCH body for updating a group.
type GroupUpdate struct {
	ID       *int     `json:"id,omitempty"`
	Name     *string  `json:"name,omitempty"`
	ZoneIDs  []int    `json:"zones,omitempty"`
	SourceID *int     `json:"source_id,omitempty"`
	Vol      *int     `json:"vol_delta,omitempty"`
	VolF     *float64 `json:"vol_f,omitempty"`
	Mute     *bool    `json:"mute,omitempty"`
}

// StreamCreate is the POST body for creating a stream.
type StreamCreate struct {
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// StreamUpdate is the PATCH body for updating a stream.
type StreamUpdate struct {
	Name   *string                `json:"name,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// PresetCreate is the POST body for creating a preset.
type PresetCreate struct {
	Name     string       `json:"name"`
	State    *PresetState `json:"state,omitempty"`
	Commands []Command    `json:"commands,omitempty"`
}

// PresetUpdate is the PATCH body for updating a preset.
type PresetUpdate struct {
	Name     *string      `json:"name,omitempty"`
	State    *PresetState `json:"state,omitempty"`
	Commands []Command    `json:"commands,omitempty"`
}

// AnnounceRequest is the POST body for making a PA announcement.
type AnnounceRequest struct {
	Media    string   `json:"media"`
	Vol      *int     `json:"vol,omitempty"`
	VolF     *float64 `json:"vol_f,omitempty"`
	VolMin   *int     `json:"vol_min,omitempty"`
	Timeout  *int     `json:"timeout,omitempty"`
	SourceID *int     `json:"source_id,omitempty"`
}
