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

// FilePlayerStream plays a local file or directory via VLC.
// Non-persistent — only needed when actively playing.
type FilePlayerStream struct {
	SubprocStream
	name string
	path string

	httpPort  int // VLC HTTP interface port
	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewFilePlayerStream creates a new file player stream.
func NewFilePlayerStream(name, path string, onChange func(models.StreamInfo)) *FilePlayerStream {
	return &FilePlayerStream{
		name:     name,
		path:     path,
		onChange: onChange,
	}
}

// Activate creates the config dir and starts VLC with HTTP interface.
func (s *FilePlayerStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("file_player: activating", "name", s.name, "path", s.path)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("file_player activate: %w", err)
	}

	device := VirtualOutputDevice(vsrc)
	path := s.path

	// HTTP interface port allocation: base 8100, 1 per vsrc
	s.httpPort = 8100 + vsrc

	s.sup = NewSupervisor("file_player/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("vlc"),
			"--intf", "http",
			"--http-host", "127.0.0.1",
			"--http-port", fmt.Sprintf("%d", s.httpPort),
			"--http-password", "amplipi",
			"--aout", "alsa",
			"--alsa-audio-device", device,
			"--no-video",
			path,
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

func (s *FilePlayerStream) Deactivate(ctx context.Context) error {
	slog.Info("file_player: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *FilePlayerStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *FilePlayerStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

func (s *FilePlayerStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("file_player: command ignored", "name", s.name, "cmd", cmd)
	return nil
}

func (s *FilePlayerStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *FilePlayerStream) IsPersistent() bool { return false }
func (s *FilePlayerStream) Type() string        { return "file_player" }

// vlcStatusResponse is a subset of VLC's HTTP status.json response.
type vlcFilePlayerStatusResponse struct {
	State       string `json:"state"` // "playing", "paused", "stopped"
	Information struct {
		Category struct {
			Meta map[string]interface{} `json:"meta"`
		} `json:"category"`
	} `json:"information"`
}

// pollVLCMetadata periodically polls VLC's HTTP interface for file metadata.
func (s *FilePlayerStream) pollVLCMetadata(ctx context.Context) {
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
			slog.Debug("file_player: metadata updated",
				"track", info.Track, "artist", info.Artist, "state", info.State)
			if s.onChange != nil {
				s.onChange(*info)
			}
		}
	}
}

// fetchVLCMetadata queries VLC's HTTP status endpoint for file metadata.
func (s *FilePlayerStream) fetchVLCMetadata(ctx context.Context) *models.StreamInfo {
	url := fmt.Sprintf("http://127.0.0.1:%d/requests/status.json", s.httpPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.SetBasicAuth("", "amplipi")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("file_player: VLC status fetch failed", "err", err)
		return nil
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var status vlcFilePlayerStatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		slog.Debug("file_player: failed to parse VLC status", "err", err)
		return nil
	}

	info := models.StreamInfo{
		Name:  s.name,
		State: parseVLCFilePlayerState(status.State),
	}

	// Extract ID3 metadata from files
	meta := status.Information.Category.Meta
	if meta != nil {
		if title, ok := meta["title"].(string); ok && title != "" {
			info.Track = title
		} else if filename, ok := meta["filename"].(string); ok && filename != "" {
			info.Track = filename
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

// parseVLCFilePlayerState converts VLC state string to StreamInfo state.
func parseVLCFilePlayerState(state string) string {
	switch state {
	case "playing":
		return "playing"
	case "paused":
		return "paused"
	case "stopped":
		return "stopped"
	default:
		return "playing"
	}
}
