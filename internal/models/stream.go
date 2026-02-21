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
