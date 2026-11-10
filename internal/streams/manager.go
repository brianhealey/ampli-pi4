package streams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

	// Per-source EQ. eqBinary is the CamillaDSP path ("" = not installed:
	// every connection uses alsaloop). eqByPhys holds the EQ to apply per
	// physical source, only for enabled EQs. conns records which kind of
	// bridge each connected vsrc got. All three under mu.
	eqBinary string
	eqByPhys map[int]*models.SourceEQ
	conns    map[int]connBridge

	// Runtime EQ state per physical source, reported by running bridges.
	// Separate lock: read by the controller while it holds its own lock.
	statusMu   sync.Mutex
	eqStatus   map[int]eqRuntime
	onEQChange func()
}

// connBridge is the bridge a connected stream's vsrc got from newBridge.
type connBridge struct {
	key string    // eqKey of the EQ it was built with; "" for alsaloop
	eq  *eqBridge // nil for alsaloop
}

// eqRuntime is the last state a bridge reported for its physical source.
type eqRuntime struct {
	owner  *eqBridge
	state  string
	reason string
}

// NewManager creates a new stream Manager.
// configDir should be ~/.config/amplipi/srcs/.
// onChange is called when a stream's metadata changes.
func NewManager(configDir string, onChange func(int, models.StreamInfo)) *Manager {
	// Set the scripts directory for binary discovery
	setStreamsScriptsDir(filepath.Join(filepath.Dir(configDir), "streams"))

	return &Manager{
		streams:   make(map[int]*StreamState),
		vsources:  NewVSRCAllocator(),
		configDir: configDir,
		onChange:  onChange,
		conns:     make(map[int]connBridge),
		eqStatus:  make(map[int]eqRuntime),
	}
}

// SetEQBinary enables per-source EQ with the given CamillaDSP binary. Empty
// (the default) means EQ is never applied and every bridge is alsaloop.
func (m *Manager) SetEQBinary(path string) {
	m.mu.Lock()
	m.eqBinary = path
	m.mu.Unlock()
}

// SetEQStatusListener registers fn to be called (from any goroutine, with no
// Manager lock held) whenever a source's runtime EQ state changes.
func (m *Manager) SetEQStatusListener(fn func()) {
	m.statusMu.Lock()
	m.onEQChange = fn
	m.statusMu.Unlock()
}

// EQStatus returns the runtime EQ state of the CamillaDSP bridge feeding
// physical source physSrc, if one is running. Takes only the status lock, so
// it is safe to call while holding other locks.
func (m *Manager) EQStatus(physSrc int) (models.EQStatus, bool) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()
	st, ok := m.eqStatus[physSrc]
	if !ok {
		return models.EQStatus{}, false
	}
	return models.EQStatus{State: st.state, Reason: st.reason}, true
}

// reportEQ records a bridge's state for its source. Entries are owned by the
// bridge that wrote them, so a bridge stopping late cannot clear the state of
// the one that replaced it.
func (m *Manager) reportEQ(b *eqBridge, state, reason string) {
	m.statusMu.Lock()
	cur, ok := m.eqStatus[b.physSrc]
	switch {
	case state == "":
		if !ok || cur.owner != b {
			m.statusMu.Unlock()
			return
		}
		delete(m.eqStatus, b.physSrc)
	case ok && cur.owner == b && cur.state == state && cur.reason == reason:
		m.statusMu.Unlock()
		return
	default:
		m.eqStatus[b.physSrc] = eqRuntime{owner: b, state: state, reason: reason}
	}
	fn := m.onEQChange
	m.statusMu.Unlock()
	if fn != nil {
		fn()
	}
}

// eqKey fingerprints an EQ setting; "" for none.
func eqKey(eq *models.SourceEQ) string {
	if eq == nil {
		return ""
	}
	b, _ := json.Marshal(eq)
	return string(b)
}

// newBridge is the bridgeFactory given to every stream: CamillaDSP when the
// destination source has an enabled EQ and CamillaDSP is installed, alsaloop
// otherwise. Called from Streamer.Connect, which only Sync calls, with m.mu
// held.
func (m *Manager) newBridge(vsrc, physSrc int) (bridge, error) {
	// The alsaloop bridge also validates that physSrc has a DAC, and is the
	// EQ bridge's fallback.
	loop, err := newALSALoopBridge(vsrc, physSrc)
	if err != nil {
		return nil, err
	}
	eq := m.eqByPhys[physSrc]
	if eq == nil {
		m.conns[vsrc] = connBridge{}
		return loop, nil
	}
	eb := newEQBridge(vsrc, physSrc, m.eqBinary, m.configDir, *eq, loop, m.reportEQ)
	m.conns[vsrc] = connBridge{key: eqKey(eq), eq: eb}
	return eb, nil
}

// effectiveEQs returns the EQ to apply per physical source: enabled EQs only,
// and none at all without CamillaDSP.
func effectiveEQs(sources []models.Source, eqBinary string) map[int]*models.SourceEQ {
	out := make(map[int]*models.SourceEQ)
	if eqBinary == "" {
		return out
	}
	for _, src := range sources {
		if src.EQ != nil && src.EQ.Enabled {
			out[src.ID] = src.EQ.Clone()
		}
	}
	return out
}

// connect connects a stream to physSrc, activating it first if needed.
// Must be called with m.mu held.
func (m *Manager) connect(ctx context.Context, id int, state *StreamState, physSrc int, name string) {
	// Activate if not yet active
	if !state.Active {
		if err := m.activateStream(ctx, state, name); err != nil {
			slog.Error("stream manager: failed to activate stream for connect", "id", id, "err", err)
			return
		}
	}

	slog.Info("stream manager: connecting stream", "id", id, "physSrc", physSrc)
	if err := state.Streamer.Connect(ctx, physSrc); err != nil {
		slog.Error("stream manager: connect error", "id", id, "physSrc", physSrc, "err", err)
		// Do not leave a stream that cannot be heard reporting "playing".
		if errors.Is(err, ErrPhysicalOutputUnavailable) && m.onChange != nil {
			m.onChange(id, models.StreamInfo{
				Name:  name,
				State: "unavailable",
				Track: err.Error(),
			})
		}
	} else {
		state.PhysSrc = physSrc
	}
}

// disconnect disconnects a connected stream. Must be called with m.mu held.
func (m *Manager) disconnect(ctx context.Context, id int, state *StreamState, what string) {
	if err := state.Streamer.Disconnect(ctx); err != nil {
		slog.Warn("stream manager: disconnect error"+what, "id", id, "err", err)
	}
	state.PhysSrc = -1
	delete(m.conns, state.VSRC)
}

// reconcileEQ brings a connected stream's bridge in line with its source's
// EQ: an EQ turned on or off swaps the bridge type (a short gap), a changed
// EQ is pushed to the running CamillaDSP. With no EQ anywhere this is a
// no-op. Must be called with m.mu held.
func (m *Manager) reconcileEQ(ctx context.Context, id int, state *StreamState, name string) {
	conn, ok := m.conns[state.VSRC]
	if !ok {
		return
	}
	want := m.eqByPhys[state.PhysSrc]
	wantKey := eqKey(want)
	if wantKey == conn.key {
		return
	}
	if (want == nil) != (conn.eq == nil) {
		physSrc := state.PhysSrc
		slog.Info("stream manager: source EQ toggled, swapping bridge", "id", id, "physSrc", physSrc, "eq", want != nil)
		m.disconnect(ctx, id, state, " before bridge swap")
		m.connect(ctx, id, state, physSrc, name)
		return
	}
	conn.eq.Update(*want)
	conn.key = wantKey
	m.conns[state.VSRC] = conn
}

// Sync reconciles the manager's running streamers with the desired model state.
// Called by Controller.apply() after every state change.
func (m *Manager) Sync(ctx context.Context, modelStreams []models.Stream, sources []models.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.eqByPhys = effectiveEQs(sources, m.eqBinary)

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

	// Step 1: Remove streams that are no longer in the model, and tear down any
	// whose config changed. A Streamer reads its config once, at construction,
	// so an edited setting only takes effect if the Streamer is rebuilt —
	// Step 2 recreates anything removed here.
	for id, state := range m.streams {
		desiredStream, desired := desiredIDs[id]
		rebuild := desired && streamConfigKey(desiredStream) != state.ConfigKey
		if !desired || rebuild {
			if rebuild {
				slog.Info("stream manager: config changed, rebuilding stream", "id", id)
			} else {
				slog.Info("stream manager: removing stream", "id", id)
			}
			if state.PhysSrc >= 0 {
				m.disconnect(ctx, id, state, " on removal")
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

	// Step 2: Add new streams from model. Walk them in ascending ID order: vsrc
	// slots are handed out in activation order, and map iteration order is
	// random, so this keeps each stream on the same vsrc across restarts.
	for _, id := range slices.Sorted(maps.Keys(desiredIDs)) {
		stream := desiredIDs[id]
		if _, exists := m.streams[id]; !exists {
			slog.Info("stream manager: adding new stream", "id", id, "type", stream.Type, "name", stream.Name)
			// Create a closure to wrap the onChange callback with the stream ID
			var streamOnChange func(models.StreamInfo)
			if m.onChange != nil {
				streamID := id // capture for closure
				streamOnChange = func(info models.StreamInfo) {
					m.onChange(streamID, info)
				}
			}
			streamer, err := NewStreamer(stream, streamOnChange)
			if err != nil {
				slog.Error("stream manager: could not create streamer", "id", id, "type", stream.Type, "err", err)
				continue
			}
			if bs, ok := streamer.(bridgeSetter); ok {
				bs.setBridgeFactory(m.newBridge)
			}
			state := &StreamState{
				Streamer:  streamer,
				StreamID:  id,
				VSRC:      -1,
				PhysSrc:   -1,
				Active:    false,
				ConfigKey: streamConfigKey(stream),
			}
			m.streams[id] = state

			// Activate persistent streams immediately
			if streamer.IsPersistent() {
				if err := m.activateStream(ctx, state, stream.Name); err != nil {
					slog.Error("stream manager: failed to activate persistent stream", "id", id, "err", err)
					// Surface the error to the API so the stream shows a clear state
					if m.onChange != nil {
						m.onChange(id, models.StreamInfo{
							Name:  stream.Name,
							State: "unavailable",
							Track: err.Error(),
						})
					}
				}
			}
		}
	}

	// Step 3: Reconcile connections for all streams, in ID order for the same
	// reason (non-persistent streams get their vsrc here).
	for _, id := range slices.Sorted(maps.Keys(m.streams)) {
		state := m.streams[id]
		desiredPhysSrc, shouldConnect := streamToPhysSrc[id]

		if shouldConnect && state.PhysSrc != desiredPhysSrc {
			// Need to connect (or reconnect to different physSrc)
			if state.PhysSrc >= 0 {
				m.disconnect(ctx, id, state, " before reconnect")
			}
			m.connect(ctx, id, state, desiredPhysSrc, desiredIDs[id].Name)

		} else if shouldConnect && state.VSRC >= 0 {
			// Connected where it should be: follow EQ changes on its source.
			m.reconcileEQ(ctx, id, state, desiredIDs[id].Name)

		} else if !shouldConnect && state.PhysSrc >= 0 {
			// Need to disconnect
			slog.Info("stream manager: disconnecting stream", "id", id)
			m.disconnect(ctx, id, state, "")

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
			m.disconnect(ctx, id, state, " on shutdown")
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
// All user-supplied configuration values are validated before creating the streamer.
// onChange is called when a stream's metadata changes (may be nil for streams that don't support it).
func NewStreamer(stream models.Stream, onChange func(models.StreamInfo)) (Streamer, error) {
	name := stream.Name

	// Validate stream name
	if err := ValidateStreamName(name); err != nil {
		return nil, fmt.Errorf("invalid stream name: %w", err)
	}

	switch stream.Type {
	case "rca":
		return NewRCAStream(name), nil

	case "aux":
		return NewAuxStream(name), nil

	case "pandora":
		username := stream.ConfigString("user")
		password := stream.ConfigString("password")
		station := stream.ConfigString("station")
		// Pandora credentials are validated by the Pandora service
		return NewPandoraStream(name, username, password, station, onChange), nil

	case "airplay":
		return NewAirPlayStream(name), nil

	case "spotify_connect", "spotify":
		return NewSpotifyStream(name, stream.ConfigInt("bitrate", 0), onChange), nil

	case "internet_radio", "internetradio":
		u := stream.ConfigString("url")
		if err := ValidateStreamURL(u); err != nil {
			return nil, fmt.Errorf("invalid stream URL: %w", err)
		}
		return NewInternetRadioStream(name, u), nil

	case "file_player", "fileplayer":
		path := stream.ConfigString("path")
		if err := ValidateFilePath(path); err != nil {
			return nil, fmt.Errorf("invalid file path: %w", err)
		}
		return NewFilePlayerStream(name, path), nil

	case "dlna":
		return NewDLNAStream(name), nil

	case "lms":
		server := stream.ConfigString("server")
		if err := ValidateLMSServer(server); err != nil {
			return nil, fmt.Errorf("invalid LMS server: %w", err)
		}
		return NewLMSStream(name, server, onChange), nil

	case "fm_radio", "fmradio":
		freq := stream.ConfigString("freq")
		if err := ValidateFrequency(freq); err != nil {
			return nil, fmt.Errorf("invalid frequency: %w", err)
		}
		return NewFMRadioStream(name, freq), nil

	case "bluetooth":
		return NewBluetoothStream(name), nil

	case "plexamp":
		return NewPlexampStream(name), nil

	default:
		return nil, fmt.Errorf("unknown stream type: %q", stream.Type)
	}
}
