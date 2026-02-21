package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// goLibrespotConfig is the go-librespot YAML config template.
const goLibrespotConfig = `device_name: "%s"
device_type: "stb"
audio_device: "%s"
external_volume: true
server:
  enabled: true
  port: %d
credentials:
  type: zeroconf
`

// SpotifyStream plays Spotify Connect audio via go-librespot.
// Persistent — go-librespot advertises on the network continuously.
type SpotifyStream struct {
	SubprocStream

	name    string
	apiPort int // 3678 + vsrc

	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewSpotifyStream creates a new Spotify Connect stream.
func NewSpotifyStream(name string, onChange func(models.StreamInfo)) *SpotifyStream {
	return &SpotifyStream{
		name:     name,
		onChange: onChange,
	}
}

// Activate writes go-librespot config and starts the process.
func (s *SpotifyStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("spotify_connect: activating", "name", s.name)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("spotify_connect activate: %w", err)
	}

	s.apiPort = 3678 + vsrc
	device := VirtualOutputDevice(vsrc)
	cfgContent := fmt.Sprintf(goLibrespotConfig, s.name, device, s.apiPort)

	if err := writeFileAtomic(dir+"/config.yml", []byte(cfgContent)); err != nil {
		return fmt.Errorf("spotify_connect: write config.yml: %w", err)
	}

	cfgDir := dir
	s.sup = NewSupervisor("spotify_connect/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("go-librespot"), "--config_dir", cfgDir)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	s.setInfo(models.StreamInfo{
		Name:  s.name,
		State: "stopped",
	})

	if err := s.activateBase(ctx, vsrc, dir); err != nil {
		return err
	}

	// Start metadata polling goroutine
	monCtx, monCancel := context.WithCancel(context.Background())
	s.monCancel = monCancel
	s.monWg.Add(1)
	go s.pollMetadata(monCtx)

	return nil
}

// Deactivate stops go-librespot and the metadata polling goroutine.
func (s *SpotifyStream) Deactivate(ctx context.Context) error {
	slog.Info("spotify_connect: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *SpotifyStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *SpotifyStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

// SendCmd sends playback commands to go-librespot via its HTTP API.
func (s *SpotifyStream) SendCmd(ctx context.Context, cmd string) error {
	var path string
	var body io.Reader
	switch cmd {
	case "play":
		path = "/player/resume"
	case "pause":
		path = "/player/pause"
	case "next":
		path = "/player/next"
		body = strings.NewReader("{}")
	case "prev":
		path = "/player/prev"
	default:
		slog.Debug("spotify_connect: unknown command", "cmd", cmd)
		return nil
	}

	url := fmt.Sprintf("http://localhost:%d%s", s.apiPort, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify_connect: command %s: %w", cmd, err)
	}
	resp.Body.Close()
	return nil
}

func (s *SpotifyStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *SpotifyStream) IsPersistent() bool { return true }
func (s *SpotifyStream) Type() string        { return "spotify_connect" }

// spotifyStatus is the JSON response from go-librespot's /status endpoint.
type spotifyStatus struct {
	PlayerState struct {
		IsPlaying bool `json:"is_playing"`
		IsPaused  bool `json:"is_paused"`
	} `json:"player_state"`
	Track struct {
		Name        string   `json:"name"`
		AlbumName   string   `json:"album_name"`
		ArtistNames []string `json:"artist_names"`
		AlbumCover  string   `json:"album_cover_url"`
	} `json:"track"`
	Stopped bool `json:"stopped"`
	Paused  bool `json:"paused"`
}

// pollMetadata periodically polls the go-librespot HTTP API for metadata.
func (s *SpotifyStream) pollMetadata(ctx context.Context) {
	defer s.monWg.Done()

	// Wait a moment before first poll to let go-librespot start
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info := s.fetchStatus(ctx)
			if info == nil {
				continue
			}
			s.setInfo(*info)
			slog.Debug("spotify_connect: metadata updated",
				"track", info.Track, "artist", info.Artist, "state", info.State)
			if s.onChange != nil {
				s.onChange(*info)
			}
		}
	}
}

// fetchStatus polls the go-librespot HTTP status endpoint.
func (s *SpotifyStream) fetchStatus(ctx context.Context) *models.StreamInfo {
	url := fmt.Sprintf("http://localhost:%d/status", s.apiPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("spotify_connect: status fetch failed", "err", err)
		return nil
	}
	defer resp.Body.Close()

	var status spotifyStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil
	}

	state := "playing"
	if status.Stopped {
		state = "stopped"
	} else if status.Paused || status.PlayerState.IsPaused {
		state = "paused"
	}

	artist := ""
	if len(status.Track.ArtistNames) > 0 {
		artist = strings.Join(status.Track.ArtistNames, ", ")
	}

	return &models.StreamInfo{
		Name:     s.name,
		State:    state,
		Track:    status.Track.Name,
		Artist:   artist,
		Album:    status.Track.AlbumName,
		ImageURL: status.Track.AlbumCover,
	}
}
