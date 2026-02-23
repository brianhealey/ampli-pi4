package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// shairportConfTemplate is the shairport-sync config file format.
// shairport-sync uses a nesting groups syntax.
const shairportConfTemplate = `general = {
    name = "%s";
    port = %d;
    udp_port_base = %d;
};
alsa = {
    output_device = "%s";
};
mpris = {
    enabled = "yes";
    title = "AmpliPi - %s";
};
`

// AirPlayStream plays AirPlay audio via shairport-sync.
// Persistent — shairport-sync must advertise on the network continuously.
type AirPlayStream struct {
	SubprocStream
	name string

	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewAirPlayStream creates a new AirPlay stream.
func NewAirPlayStream(name string, onChange func(models.StreamInfo)) *AirPlayStream {
	return &AirPlayStream{
		name:     name,
		onChange: onChange,
	}
}

// Activate writes the shairport-sync config and starts the process.
func (s *AirPlayStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("airplay: activating", "name", s.name)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("airplay activate: %w", err)
	}

	confPath := dir + "/shairport.conf"

	// Port allocation: base 5100, 100 per vsrc
	port := 5100 + 100*vsrc
	udpBase := 6101 + 100*vsrc
	device := VirtualOutputDevice(vsrc)

	cfgContent := fmt.Sprintf(shairportConfTemplate, s.name, port, udpBase, device, s.name)
	if err := writeFileAtomic(confPath, []byte(cfgContent)); err != nil {
		return fmt.Errorf("airplay: write shairport.conf: %w", err)
	}

	s.sup = NewSupervisor("airplay/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("shairport-sync"), "-c", confPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	s.setInfo(models.StreamInfo{
		Name:  s.name,
		State: "connected",
	})

	if err := s.activateBase(ctx, vsrc, dir); err != nil {
		return err
	}

	// Start MPRIS metadata monitoring goroutine
	monCtx, monCancel := context.WithCancel(context.Background())
	s.monCancel = monCancel
	s.monWg.Add(1)
	go s.pollMPRISMetadata(monCtx)

	return nil
}

func (s *AirPlayStream) Deactivate(ctx context.Context) error {
	slog.Info("airplay: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *AirPlayStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *AirPlayStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

// SendCmd handles AirPlay playback controls.
// In v1, MPRIS/D-Bus integration is not implemented — commands are ignored.
func (s *AirPlayStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("airplay: command (no-op in v1)", "name", s.name, "cmd", cmd)
	return nil
}

func (s *AirPlayStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *AirPlayStream) IsPersistent() bool { return true }
func (s *AirPlayStream) Type() string        { return "airplay" }

// pollMPRISMetadata monitors shairport-sync MPRIS D-Bus interface for metadata changes.
func (s *AirPlayStream) pollMPRISMetadata(ctx context.Context) {
	defer s.monWg.Done()

	// Wait for shairport-sync to start and register on D-Bus
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info := s.fetchMPRISMetadata(ctx)
			if info == nil {
				continue
			}
			s.setInfo(*info)
			slog.Debug("airplay: metadata updated",
				"track", info.Track, "artist", info.Artist, "state", info.State)
			if s.onChange != nil {
				s.onChange(*info)
			}
		}
	}
}

// fetchMPRISMetadata queries the MPRIS D-Bus interface for current playback metadata.
func (s *AirPlayStream) fetchMPRISMetadata(ctx context.Context) *models.StreamInfo {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		slog.Debug("airplay: failed to connect to D-Bus", "err", err)
		return nil
	}
	defer conn.Close()

	// MPRIS interface name: org.mpris.MediaPlayer2.shairport_sync
	// Note: D-Bus converts hyphens to underscores in service names
	obj := conn.Object("org.mpris.MediaPlayer2.shairport_sync", "/org/mpris/MediaPlayer2")

	// Get PlaybackStatus property
	playbackStatus, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.PlaybackStatus")
	if err != nil {
		slog.Debug("airplay: failed to get playback status", "err", err)
		return &models.StreamInfo{
			Name:  s.name,
			State: "stopped",
		}
	}

	// Get Metadata property
	metadataVariant, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Metadata")
	if err != nil {
		slog.Debug("airplay: failed to get metadata", "err", err)
		return nil
	}

	// Parse metadata map
	metadata, ok := metadataVariant.Value().(map[string]dbus.Variant)
	if !ok {
		return nil
	}

	// Extract fields from metadata
	info := models.StreamInfo{
		Name:  s.name,
		State: parsePlaybackState(playbackStatus.Value()),
	}

	// Title: xesam:title
	if title, ok := metadata["xesam:title"]; ok {
		if titleStr, ok := title.Value().(string); ok {
			info.Track = titleStr
		}
	}

	// Artist: xesam:artist (array of strings)
	if artist, ok := metadata["xesam:artist"]; ok {
		if artistArr, ok := artist.Value().([]string); ok && len(artistArr) > 0 {
			info.Artist = artistArr[0]
		}
	}

	// Album: xesam:album
	if album, ok := metadata["xesam:album"]; ok {
		if albumStr, ok := album.Value().(string); ok {
			info.Album = albumStr
		}
	}

	// Album Art: mpris:artUrl
	if artURL, ok := metadata["mpris:artUrl"]; ok {
		if artStr, ok := artURL.Value().(string); ok {
			info.ImageURL = artStr
		}
	}

	return &info
}

// parsePlaybackState converts MPRIS PlaybackStatus to StreamInfo state.
func parsePlaybackState(status interface{}) string {
	if statusStr, ok := status.(string); ok {
		switch statusStr {
		case "Playing":
			return "playing"
		case "Paused":
			return "paused"
		case "Stopped":
			return "stopped"
		}
	}
	return "stopped"
}
