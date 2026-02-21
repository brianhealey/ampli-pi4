package models

// BrowsableItem represents an item that can be browsed in a stream (station, playlist, etc.)
type BrowsableItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // "station" | "playlist" | "album" | "track" | "folder"
	Thumbnail string `json:"thumbnail,omitempty"`
}

// BrowseResponse is the response body for stream browse requests.
type BrowseResponse struct {
	Items []BrowsableItem `json:"items"`
}

// StreamCommand represents a command to send to a stream.
type StreamCommand struct {
	Command string `json:"cmd"`
}

// Known stream types.
const (
	StreamTypePandora       = "pandora"
	StreamTypeAirPlay       = "airplay"
	StreamTypeSpotify       = "spotify"
	StreamTypeDLNA          = "dlna"
	StreamTypeInternetRadio = "internetradio"
	StreamTypeFMRadio       = "fmradio"
	StreamTypeLMS           = "lms"
	StreamTypeBluetooth     = "bluetooth"
	StreamTypeRCA           = "rca"
	StreamTypeAux           = "aux"
	StreamTypeFileplayer    = "fileplayer"
)

// Special stream IDs from Python defaults.
const (
	AuxStreamID = 995
	RCAStream0  = 996
	RCAStream1  = 997
	RCAStream2  = 998
	RCAStream3  = 999
)

// ConfigString extracts a string config field safely.
func (s *Stream) ConfigString(key string) string {
	if s.Config == nil {
		return ""
	}
	v, _ := s.Config[key].(string)
	return v
}

// ConfigInt extracts an int config field safely.
// Returns def if the key is missing or not an integer.
func (s *Stream) ConfigInt(key string, def int) int {
	if s.Config == nil {
		return def
	}
	switch v := s.Config[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return def
}
