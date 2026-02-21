package streams

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// LMSStream streams from a Logitech Media Server using squeezelite.
// Persistent — squeezelite maintains a continuous connection to the LMS server.
type LMSStream struct {
	SubprocStream

	name   string
	server string // LMS server IP, empty = auto-discover

	lmsServer string // resolved server (possibly discovered)

	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewLMSStream creates a new LMS stream.
func NewLMSStream(name, server string, onChange func(models.StreamInfo)) *LMSStream {
	return &LMSStream{
		name:     name,
		server:   server,
		onChange: onChange,
	}
}

// lmsMACAddress generates a stable MAC address from the stream name
// using MD5 hashing (matching the Python implementation).
func lmsMACAddress(name string) string {
	hash := md5.Sum([]byte(name))
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		hash[0], hash[1], hash[2], hash[3], hash[4], hash[5])
}

// Activate starts squeezelite and the metadata polling goroutine.
func (s *LMSStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("lms: activating", "name", s.name, "server", s.server)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("lms activate: %w", err)
	}

	// Determine server (may be auto-discovered at runtime by squeezelite)
	s.lmsServer = s.server
	if s.lmsServer == "" {
		s.lmsServer = discoverLMSServer()
	}

	mac := lmsMACAddress(s.name)
	device := VirtualOutputDevice(vsrc)
	name := s.name
	server := s.server

	s.sup = NewSupervisor("lms/"+s.name, func() *exec.Cmd {
		args := []string{
			"-n", name,
			"-m", mac,
			"-o", device,
		}
		if server != "" {
			args = append(args, "-s", server)
		}
		cmd := exec.Command(findBinary("squeezelite"), args...)
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

// Deactivate stops squeezelite and the metadata goroutine.
func (s *LMSStream) Deactivate(ctx context.Context) error {
	slog.Info("lms: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *LMSStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *LMSStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

func (s *LMSStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("lms: command (not implemented)", "name", s.name, "cmd", cmd)
	return nil
}

func (s *LMSStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *LMSStream) IsPersistent() bool { return true }
func (s *LMSStream) Type() string        { return "lms" }

// discoverLMSServer tries to run find_lms_server and parse its stdout.
// Returns empty string if discovery fails (squeezelite will auto-discover).
func discoverLMSServer() string {
	if streamsScriptsDir == "" {
		return ""
	}
	binary := findBinary("find_lms_server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// lmsStatusResponse is a subset of the LMS JSON status API.
type lmsStatusResponse struct {
	PlayerName string `json:"player_name"`
	Mode       string `json:"mode"` // "play", "pause", "stop"
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	ArtworkURL string `json:"artwork_url"`
}

// pollMetadata periodically polls the LMS server for playback metadata.
func (s *LMSStream) pollMetadata(ctx context.Context) {
	defer s.monWg.Done()

	// Give squeezelite time to connect before polling
	select {
	case <-ctx.Done():
		return
	case <-time.After(8 * time.Second):
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			srv := s.lmsServer
			if srv == "" {
				continue
			}
			info := s.fetchLMSStatus(ctx, srv)
			if info == nil {
				continue
			}
			s.setInfo(*info)
			slog.Debug("lms: metadata updated",
				"track", info.Track, "artist", info.Artist, "state", info.State)
			if s.onChange != nil {
				s.onChange(*info)
			}
		}
	}
}

// fetchLMSStatus polls the LMS HTTP status endpoint for the named player.
func (s *LMSStream) fetchLMSStatus(ctx context.Context, server string) *models.StreamInfo {
	rawURL := fmt.Sprintf("http://%s:9000/status.html", server)
	params := url.Values{}
	params.Set("player", s.name)
	params.Set("type", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("lms: status fetch failed", "err", err)
		return nil
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var status lmsStatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		return nil
	}

	state := "stopped"
	switch status.Mode {
	case "play":
		state = "playing"
	case "pause":
		state = "paused"
	}

	return &models.StreamInfo{
		Name:     s.name,
		State:    state,
		Track:    status.Title,
		Artist:   status.Artist,
		Album:    status.Album,
		ImageURL: status.ArtworkURL,
	}
}
