// Package streams implements the AmpliPi stream subsystem.
// Each stream type (Pandora, AirPlay, Spotify, etc.) is a Streamer.
// The Manager owns all Streamers and coordinates their lifecycle.
package streams

import (
	"context"
	"errors"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// ErrNotSupported is returned by stream types that are not yet implemented.
var ErrNotSupported = errors.New("stream type not supported")

// Streamer is the interface every stream type must implement.
// Implementations are NOT required to be thread-safe internally;
// the Manager serializes calls to each Streamer.
type Streamer interface {
	// Activate allocates a virtual source and starts the subprocess.
	// vsrc is the ALSA loopback index (0-11).
	// configDir is the per-stream config path (~/.config/amplipi/srcs/v{vsrc}/).
	Activate(ctx context.Context, vsrc int, configDir string) error

	// Deactivate stops the subprocess and frees resources.
	// Does NOT free the vsrc — caller (Manager) does that.
	Deactivate(ctx context.Context) error

	// Connect routes audio from vsrc to physSrc via alsaloop.
	// Called after Activate when a source is assigned this stream.
	Connect(ctx context.Context, physSrc int) error

	// Disconnect stops the alsaloop connection.
	// Stream subprocess keeps running (persistent streams).
	Disconnect(ctx context.Context) error

	// SendCmd delivers a control command to the stream.
	// Common: "play", "pause", "next", "prev"
	// Stream-specific: "love", "ban", "station=<id>", etc.
	SendCmd(ctx context.Context, cmd string) error

	// Info returns the current playback metadata (thread-safe).
	Info() models.StreamInfo

	// IsPersistent returns true if the stream subprocess should keep
	// running even when not physically routed to a source.
	IsPersistent() bool

	// Type returns the stream type string ("pandora", "airplay", etc.)
	Type() string
}

// StreamState tracks a Streamer's runtime state within the Manager.
type StreamState struct {
	Streamer Streamer
	StreamID int
	VSRC     int // -1 if not activated
	PhysSrc  int // -1 if not connected
	Active   bool
}
