package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// Manager owns all Streamers and coordinates their lifecycle.
// All exported methods are safe to call concurrently.
type Manager struct {
	mu        sync.Mutex
	streams   map[int]*StreamState // stream model ID → state
	vsources  *VSRCAllocator
	configDir string // ~/.config/amplipi/srcs/
	onChange  func(streamID int, info models.StreamInfo)
}

// NewManager creates a new stream Manager.
// configDir should be ~/.config/amplipi/srcs/.
// onChange is called when a stream's metadata changes.
func NewManager(configDir string, onChange func(int, models.StreamInfo)) *Manager {
	// Set the scripts directory for binary discovery
	streamsScriptsDir = filepath.Join(filepath.Dir(configDir), "streams")

	return &Manager{
		streams:   make(map[int]*StreamState),
		vsources:  NewVSRCAllocator(),
		configDir: configDir,
		onChange:  onChange,
	}
}

// Sync reconciles the manager's running streamers with the desired model state.
// Called by Controller.apply() after every state change.
func (m *Manager) Sync(ctx context.Context, modelStreams []models.Stream, sources []models.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a map of streamID → physSrc from the sources configuration
	streamToPhysSrc := make(map[int]int)
	for _, src := range sources {
		if strings.HasPrefix(src.Input, "stream=") {
			idStr := strings.TrimPrefix(src.Input, "stream=")
			id, err := strconv.Atoi(idStr)
			if err == nil {
				streamToPhysSrc[id] = src.ID
			}
		}
	}

	// Build a set of desired stream IDs
	desiredIDs := make(map[int]models.Stream, len(modelStreams))
	for _, s := range modelStreams {
		desiredIDs[s.ID] = s
	}

	// Step 1: Remove streams that are no longer in the model
	for id, state := range m.streams {
		if _, desired := desiredIDs[id]; !desired {
			slog.Info("stream manager: removing stream", "id", id)
			if state.PhysSrc >= 0 {
				if err := state.Streamer.Disconnect(ctx); err != nil {
					slog.Warn("stream manager: disconnect error on removal", "id", id, "err", err)
				}
			}
			if state.Active {
				if err := state.Streamer.Deactivate(ctx); err != nil {
					slog.Warn("stream manager: deactivate error on removal", "id", id, "err", err)
				}
				if state.VSRC >= 0 {
					m.vsources.Free(state.VSRC)
				}
			}
			delete(m.streams, id)
		}
	}

	// Step 2: Add new streams from model
	for id, stream := range desiredIDs {
		if _, exists := m.streams[id]; !exists {
			slog.Info("stream manager: adding new stream", "id", id, "type", stream.Type, "name", stream.Name)
			streamer, err := NewStreamer(stream)
			if err != nil {
				slog.Error("stream manager: could not create streamer", "id", id, "type", stream.Type, "err", err)
				continue
			}
			state := &StreamState{
				Streamer: streamer,
				StreamID: id,
				VSRC:     -1,
				PhysSrc:  -1,
				Active:   false,
			}
			m.streams[id] = state

			// Activate persistent streams immediately
			if streamer.IsPersistent() {
				if err := m.activateStream(ctx, state, stream.Name); err != nil {
					slog.Error("stream manager: failed to activate persistent stream", "id", id, "err", err)
				}
			}
		}
	}

	// Step 3: Reconcile connections for all streams
	for id, state := range m.streams {
		desiredPhysSrc, shouldConnect := streamToPhysSrc[id]

		if shouldConnect && state.PhysSrc != desiredPhysSrc {
			// Need to connect (or reconnect to different physSrc)
			if state.PhysSrc >= 0 {
				if err := state.Streamer.Disconnect(ctx); err != nil {
					slog.Warn("stream manager: disconnect error before reconnect", "id", id, "err", err)
				}
				state.PhysSrc = -1
			}

			// Activate if not yet active
			if !state.Active {
				if err := m.activateStream(ctx, state, desiredIDs[id].Name); err != nil {
					slog.Error("stream manager: failed to activate stream for connect", "id", id, "err", err)
					continue
				}
			}

			slog.Info("stream manager: connecting stream", "id", id, "physSrc", desiredPhysSrc)
			if err := state.Streamer.Connect(ctx, desiredPhysSrc); err != nil {
				slog.Warn("stream manager: connect error", "id", id, "physSrc", desiredPhysSrc, "err", err)
			} else {
				state.PhysSrc = desiredPhysSrc
			}

		} else if !shouldConnect && state.PhysSrc >= 0 {
			// Need to disconnect
			slog.Info("stream manager: disconnecting stream", "id", id)
			if err := state.Streamer.Disconnect(ctx); err != nil {
				slog.Warn("stream manager: disconnect error", "id", id, "err", err)
			}
			state.PhysSrc = -1

			// Deactivate non-persistent streams when disconnected
			if !state.Streamer.IsPersistent() && state.Active {
				if err := state.Streamer.Deactivate(ctx); err != nil {
					slog.Warn("stream manager: deactivate error", "id", id, "err", err)
				}
				if state.VSRC >= 0 {
					m.vsources.Free(state.VSRC)
					state.VSRC = -1
				}
				state.Active = false
			}
		}
	}

	return nil
}

// activateStream allocates a vsrc (if needed) and calls Activate on the streamer.
// Must be called with m.mu held.
func (m *Manager) activateStream(ctx context.Context, state *StreamState, name string) error {
	if state.Active {
		return nil
	}

	vsrc := -1
	configDir := m.configDir

	// Hardware passthrough streams (rca, aux) don't need a vsrc
	if streamNeedsVSRC(state.Streamer) {
		var err error
		vsrc, err = m.vsources.Alloc()
		if err != nil {
			return fmt.Errorf("no vsrc available for stream %q: %w", name, err)
		}
		// Build per-stream config dir
		streamConfigDir := filepath.Join(configDir, fmt.Sprintf("v%d", vsrc))
		if err := os.MkdirAll(streamConfigDir, 0755); err != nil {
			m.vsources.Free(vsrc)
			return fmt.Errorf("mkdir stream config dir: %w", err)
		}
		configDir = streamConfigDir
	}

	if err := state.Streamer.Activate(ctx, vsrc, configDir); err != nil {
		if vsrc >= 0 {
			m.vsources.Free(vsrc)
		}
		return fmt.Errorf("activate: %w", err)
	}

	state.VSRC = vsrc
	state.Active = true
	slog.Info("stream manager: activated stream", "name", name, "vsrc", vsrc)
	return nil
}

// streamNeedsVSRC returns false for hardware passthrough streams that don't
// need an ALSA virtual source slot.
func streamNeedsVSRC(s Streamer) bool {
	switch s.Type() {
	case "rca", "aux", "plexamp":
		return false
	}
	return true
}

// SendCmd delivers a command to a stream by model ID.
func (m *Manager) SendCmd(ctx context.Context, streamID int, cmd string) error {
	m.mu.Lock()
	state, ok := m.streams[streamID]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("stream %d not found", streamID)
	}
	return state.Streamer.SendCmd(ctx, cmd)
}

// Info returns the current StreamInfo for a stream, or nil if not found.
func (m *Manager) Info(streamID int) *models.StreamInfo {
	m.mu.Lock()
	state, ok := m.streams[streamID]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	info := state.Streamer.Info()
	return &info
}

// Shutdown deactivates all streams cleanly.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Info("stream manager: shutting down", "count", len(m.streams))
	for id, state := range m.streams {
		if state.PhysSrc >= 0 {
			if err := state.Streamer.Disconnect(ctx); err != nil {
				slog.Warn("stream manager: disconnect error on shutdown", "id", id, "err", err)
			}
		}
		if state.Active {
			if err := state.Streamer.Deactivate(ctx); err != nil {
				slog.Warn("stream manager: deactivate error on shutdown", "id", id, "err", err)
			}
			if state.VSRC >= 0 {
				m.vsources.Free(state.VSRC)
			}
		}
		delete(m.streams, id)
	}
	return nil
}

// NewStreamer creates the correct Streamer implementation for a stream model.
func NewStreamer(stream models.Stream) (Streamer, error) {
	name := stream.Name

	switch stream.Type {
	case "rca":
		return NewRCAStream(name), nil

	case "aux":
		return NewAuxStream(name), nil

	case "pandora":
		username := stream.ConfigString("user")
		password := stream.ConfigString("password")
		station := stream.ConfigString("station")
		return NewPandoraStream(name, username, password, station, nil), nil

	case "airplay":
		return NewAirPlayStream(name), nil

	case "spotify_connect", "spotify":
		return NewSpotifyStream(name, nil), nil

	case "internet_radio", "internetradio":
		u := stream.ConfigString("url")
		return NewInternetRadioStream(name, u), nil

	case "file_player", "fileplayer":
		path := stream.ConfigString("path")
		return NewFilePlayerStream(name, path), nil

	case "dlna":
		return NewDLNAStream(name), nil

	case "lms":
		server := stream.ConfigString("server")
		return NewLMSStream(name, server, nil), nil

	case "fm_radio", "fmradio":
		freq := stream.ConfigString("freq")
		return NewFMRadioStream(name, freq), nil

	case "bluetooth":
		return NewBluetoothStream(name), nil

	case "plexamp":
		return NewPlexampStream(name), nil

	default:
		return nil, fmt.Errorf("unknown stream type: %q", stream.Type)
	}
}
