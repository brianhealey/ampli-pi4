package streams

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// InternetRadioStream plays an internet radio stream via VLC.
// It is persistent — VLC keeps the connection alive even when not physically routed.
type InternetRadioStream struct {
	SubprocStream
	url  string
	name string

	httpPort  int // VLC HTTP interface port
	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewInternetRadioStream creates a new internet radio stream.
func NewInternetRadioStream(name, url string, onChange func(models.StreamInfo)) *InternetRadioStream {
	return &InternetRadioStream{
		url:      url,
		name:     name,
		onChange: onChange,
	}
}

// Activate creates the config dir and starts VLC with HTTP interface.
func (s *InternetRadioStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("internet_radio: activating", "name", s.name, "url", s.url)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("internet_radio activate: %w", err)
	}

	device := VirtualOutputDevice(vsrc)
	url := s.url

	// HTTP interface port allocation: base 8090, 1 per vsrc
	s.httpPort = 8090 + vsrc

	s.sup = NewSupervisor("internet_radio/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("vlc"),
			"--intf", "http",
			"--http-host", "127.0.0.1",
			"--http-port", fmt.Sprintf("%d", s.httpPort),
			"--http-password", "amplipi",
			"--aout", "alsa",
			"--alsa-audio-device", device,
			"--no-video",
			url,
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	s.setInfo(models.StreamInfo{Name: s.name, State: "playing"})

	if err := s.activateBase(ctx, vsrc, dir); err != nil {
		return err
	}

	// Start VLC HTTP metadata polling goroutine
	monCtx, monCancel := context.WithCancel(context.Background())
	s.monCancel = monCancel
	s.monWg.Add(1)
	go s.pollVLCMetadata(monCtx)

	return nil
}

func (s *InternetRadioStream) Deactivate(ctx context.Context) error {
	slog.Info("internet_radio: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *InternetRadioStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *InternetRadioStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

func (s *InternetRadioStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("internet_radio: command ignored", "name", s.name, "cmd", cmd)
	return nil
}

func (s *InternetRadioStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *InternetRadioStream) IsPersistent() bool { return true }
func (s *InternetRadioStream) Type() string        { return "internet_radio" }

// vlcStatusResponse is a subset of VLC's HTTP status.json response.
type vlcStatusResponse struct {
	State       string `json:"state"`       // "playing", "paused", "stopped"
	Information struct {
		Category struct {
			Meta map[string]interface{} `json:"meta"`
		} `json:"category"`
	} `json:"information"`
}

// pollVLCMetadata periodically polls VLC's HTTP interface for metadata.
func (s *InternetRadioStream) pollVLCMetadata(ctx context.Context) {
	defer s.monWg.Done()

	// Wait for VLC to start HTTP interface
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
			info := s.fetchVLCMetadata(ctx)
			if info == nil {
				continue
			}
			s.setInfo(*info)
			slog.Debug("internet_radio: metadata updated",
				"track", info.Track, "artist", info.Artist, "state", info.State)
			if s.onChange != nil {
				s.onChange(*info)
			}
		}
	}
}

// fetchVLCMetadata queries VLC's HTTP status endpoint for metadata.
func (s *InternetRadioStream) fetchVLCMetadata(ctx context.Context) *models.StreamInfo {
	url := fmt.Sprintf("http://127.0.0.1:%d/requests/status.json", s.httpPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.SetBasicAuth("", "amplipi")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("internet_radio: VLC status fetch failed", "err", err)
		return nil
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var status vlcStatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		slog.Debug("internet_radio: failed to parse VLC status", "err", err)
		return nil
	}

	info := models.StreamInfo{
		Name:  s.name,
		State: parseVLCState(status.State),
	}

	// Extract metadata from the meta map
	meta := status.Information.Category.Meta
	if meta != nil {
		// Try various metadata field names (ICY metadata, ID3 tags, etc.)
		if title, ok := meta["title"].(string); ok && title != "" {
			info.Track = title
		} else if nowPlaying, ok := meta["now_playing"].(string); ok && nowPlaying != "" {
			info.Track = nowPlaying
		}

		if artist, ok := meta["artist"].(string); ok && artist != "" {
			info.Artist = artist
		}

		if album, ok := meta["album"].(string); ok && album != "" {
			info.Album = album
		}

		if artURL, ok := meta["artwork_url"].(string); ok && artURL != "" {
			info.ImageURL = artURL
		}
	}

	return &info
}

// parseVLCState converts VLC state string to StreamInfo state.
func parseVLCState(state string) string {
	switch state {
	case "playing":
		return "playing"
	case "paused":
		return "paused"
	case "stopped":
		return "stopped"
	default:
		return "playing" // Default to playing for internet radio
	}
}
