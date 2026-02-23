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

// BluetoothStream receives audio from a Bluetooth A2DP source via bluealsa.
// Persistent — must remain discoverable and connected between source switches.
type BluetoothStream struct {
	SubprocStream
	name string

	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewBluetoothStream creates a new Bluetooth stream.
func NewBluetoothStream(name string, onChange func(models.StreamInfo)) *BluetoothStream {
	return &BluetoothStream{
		name:     name,
		onChange: onChange,
	}
}

// Activate starts bluealsa-aplay which forwards Bluetooth A2DP audio to ALSA.
func (s *BluetoothStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("bluetooth: activating", "name", s.name)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("bluetooth activate: %w", err)
	}

	device := VirtualOutputDevice(vsrc)

	s.sup = NewSupervisor("bluetooth/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("bluealsa-aplay"),
			"--profile=a2dp",
			"-D", "bluealsa",
			device,
		)
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

	// Start BlueZ AVRCP metadata monitoring goroutine
	monCtx, monCancel := context.WithCancel(context.Background())
	s.monCancel = monCancel
	s.monWg.Add(1)
	go s.pollBluetoothMetadata(monCtx)

	return nil
}

func (s *BluetoothStream) Deactivate(ctx context.Context) error {
	slog.Info("bluetooth: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *BluetoothStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *BluetoothStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

// SendCmd handles Bluetooth playback controls.
// AVRCP command relay is not implemented in v1.
func (s *BluetoothStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("bluetooth: command (not implemented in v1)", "name", s.name, "cmd", cmd)
	return nil
}

func (s *BluetoothStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *BluetoothStream) IsPersistent() bool { return true }
func (s *BluetoothStream) Type() string        { return "bluetooth" }

// pollBluetoothMetadata monitors BlueZ MediaPlayer1 D-Bus interface for AVRCP metadata.
func (s *BluetoothStream) pollBluetoothMetadata(ctx context.Context) {
	defer s.monWg.Done()

	// Wait for Bluetooth connection to establish
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
			info := s.fetchBluetoothMetadata(ctx)
			if info == nil {
				continue
			}
			s.setInfo(*info)
			slog.Debug("bluetooth: metadata updated",
				"track", info.Track, "artist", info.Artist, "state", info.State)
			if s.onChange != nil {
				s.onChange(*info)
			}
		}
	}
}

// fetchBluetoothMetadata queries BlueZ MediaPlayer1 interface for AVRCP metadata.
func (s *BluetoothStream) fetchBluetoothMetadata(ctx context.Context) *models.StreamInfo {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		slog.Debug("bluetooth: failed to connect to D-Bus", "err", err)
		return nil
	}
	defer conn.Close()

	// Find connected Bluetooth media players
	// BlueZ exposes media players at /org/bluez/hci0/dev_XX_XX_XX_XX_XX_XX/playerX
	obj := conn.Object("org.bluez", "/")
	call := obj.Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0)
	if call.Err != nil {
		slog.Debug("bluetooth: failed to get managed objects", "err", call.Err)
		return nil
	}

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := call.Store(&objects); err != nil {
		return nil
	}

	// Find first media player
	var playerPath dbus.ObjectPath
	for path, interfaces := range objects {
		if _, ok := interfaces["org.bluez.MediaPlayer1"]; ok {
			playerPath = path
			break
		}
	}

	if playerPath == "" {
		// No connected media player
		return &models.StreamInfo{
			Name:  s.name,
			State: "stopped",
		}
	}

	// Get MediaPlayer1 properties
	playerObj := conn.Object("org.bluez", playerPath)

	// Get Track property (metadata)
	trackVariant, err := playerObj.GetProperty("org.bluez.MediaPlayer1.Track")
	if err != nil {
		slog.Debug("bluetooth: failed to get Track property", "err", err)
		return nil
	}

	// Get Status property (playing/paused/stopped)
	statusVariant, err := playerObj.GetProperty("org.bluez.MediaPlayer1.Status")
	if err != nil {
		slog.Debug("bluetooth: failed to get Status property", "err", err)
		return nil
	}

	info := models.StreamInfo{
		Name:  s.name,
		State: parseBluetoothStatus(statusVariant.Value()),
	}

	// Parse track metadata
	if trackMap, ok := trackVariant.Value().(map[string]dbus.Variant); ok {
		if title, ok := trackMap["Title"]; ok {
			if titleStr, ok := title.Value().(string); ok {
				info.Track = titleStr
			}
		}
		if artist, ok := trackMap["Artist"]; ok {
			if artistStr, ok := artist.Value().(string); ok {
				info.Artist = artistStr
			}
		}
		if album, ok := trackMap["Album"]; ok {
			if albumStr, ok := album.Value().(string); ok {
				info.Album = albumStr
			}
		}
	}

	return &info
}

// parseBluetoothStatus converts BlueZ MediaPlayer1 status to StreamInfo state.
func parseBluetoothStatus(status interface{}) string {
	if statusStr, ok := status.(string); ok {
		switch statusStr {
		case "playing":
			return "playing"
		case "paused":
			return "paused"
		case "stopped":
			return "stopped"
		}
	}
	return "stopped"
}
