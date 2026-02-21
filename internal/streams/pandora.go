package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// eventcmdContent is a minimal pianobar event handler that writes the
// current song metadata to the currentSong file.
const eventcmdContent = `#!/bin/bash
# Minimal pianobar event handler for AmpliPi
SONGFILE="$(dirname "$0")/currentSong"
case "$1" in
    songstart)
        echo "${title},,,${artist},,,${album},,,${coverArt},,,${rating},,,${stationName}" > "$SONGFILE"
        ;;
    usergetstations)
        ;;
esac
exit 0
`

// pianobarConfig is the template for pianobar's config file.
const pianobarConfig = `user = %s
password = %s
event_command = %s
fifo = %s
audio_driver = alsa
audio_device = %s
`

// PandoraStream plays Pandora internet radio using pianobar.
type PandoraStream struct {
	SubprocStream

	username string
	password string
	station  string

	fifoPath        string
	currentSongPath string

	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewPandoraStream creates a new Pandora stream.
func NewPandoraStream(name, username, password, station string, onChange func(models.StreamInfo)) *PandoraStream {
	return &PandoraStream{
		username: username,
		password: password,
		station:  station,
		onChange: onChange,
	}
}

// Activate writes pianobar config, creates FIFO, and starts pianobar.
func (s *PandoraStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("pandora: activating", "username", s.username)

	// Create pianobar config directory inside configDir
	pianobarDir := filepath.Join(configDir, ".config", "pianobar")
	if err := os.MkdirAll(pianobarDir, 0755); err != nil {
		return fmt.Errorf("pandora: mkdir pianobar config dir: %w", err)
	}

	// Paths
	eventcmdPath := filepath.Join(pianobarDir, "eventcmd.sh")
	fifoPath := filepath.Join(pianobarDir, "ctl")
	currentSongPath := filepath.Join(pianobarDir, "currentSong")
	audioDevice := VirtualOutputDevice(vsrc)

	// Write eventcmd.sh
	if err := os.WriteFile(eventcmdPath, []byte(eventcmdContent), 0755); err != nil {
		return fmt.Errorf("pandora: write eventcmd.sh: %w", err)
	}

	// Try to copy eventcmd.sh from the streams scripts dir (override if exists)
	if streamsScriptsDir != "" {
		src := filepath.Join(streamsScriptsDir, "eventcmd.sh")
		if data, err := os.ReadFile(src); err == nil {
			if err2 := os.WriteFile(eventcmdPath, data, 0755); err2 != nil {
				slog.Warn("pandora: could not copy eventcmd.sh from scripts dir", "err", err2)
			}
		}
	}

	// Write pianobar config file
	cfgContent := fmt.Sprintf(pianobarConfig,
		s.username,
		s.password,
		eventcmdPath,
		fifoPath,
		audioDevice,
	)
	if err := writeFileAtomic(filepath.Join(pianobarDir, "config"), []byte(cfgContent)); err != nil {
		return fmt.Errorf("pandora: write config: %w", err)
	}

	// Create control FIFO (ignore EEXIST)
	_ = os.Remove(fifoPath)
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		return fmt.Errorf("pandora: mkfifo: %w", err)
	}

	s.fifoPath = fifoPath
	s.currentSongPath = currentSongPath

	// Start supervisor for pianobar
	// Pianobar uses HOME to find its config; we set HOME to configDir's parent
	// so it reads <configDir>/.config/pianobar/config
	homeDir := configDir
	s.sup = NewSupervisor("pandora/"+s.username, func() *exec.Cmd {
		cmd := exec.Command(findBinary("pianobar"))
		cmd.Dir = pianobarDir
		cmd.Env = append(os.Environ(), "HOME="+homeDir)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	s.setInfo(models.StreamInfo{
		Name:  "Pandora",
		State: "connected",
	})

	if err := s.activateBase(ctx, vsrc, configDir); err != nil {
		return err
	}

	// Start metadata monitor goroutine
	monCtx, monCancel := context.WithCancel(context.Background())
	s.monCancel = monCancel
	s.monWg.Add(1)
	go s.monitorCurrentSong(monCtx, currentSongPath)

	// Auto-select station if specified
	if s.station != "" {
		// Give pianobar time to start before sending station command
		go func() {
			select {
			case <-monCtx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			if err := s.writeToFIFO("s\n" + s.station + "\n"); err != nil {
				slog.Warn("pandora: failed to send initial station", "err", err)
			}
		}()
	}

	return nil
}

// Deactivate stops pianobar and the monitor goroutine.
func (s *PandoraStream) Deactivate(ctx context.Context) error {
	slog.Info("pandora: deactivating")
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *PandoraStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *PandoraStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

// SendCmd delivers a command to pianobar via the control FIFO.
func (s *PandoraStream) SendCmd(_ context.Context, cmd string) error {
	var fifoCmd string
	switch {
	case cmd == "play":
		fifoCmd = "P\n"
	case cmd == "pause":
		fifoCmd = "S\n"
	case cmd == "next":
		fifoCmd = "n\n"
	case cmd == "love":
		fifoCmd = "+\n"
	case cmd == "ban":
		fifoCmd = "-\n"
	case cmd == "shelve":
		fifoCmd = "t\n"
	case strings.HasPrefix(cmd, "station="):
		id := strings.TrimPrefix(cmd, "station=")
		fifoCmd = "s\n" + id + "\n"
	default:
		slog.Debug("pandora: unknown command", "cmd", cmd)
		return nil
	}
	return s.writeToFIFO(fifoCmd)
}

func (s *PandoraStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *PandoraStream) IsPersistent() bool { return true }
func (s *PandoraStream) Type() string        { return "pandora" }

// writeToFIFO writes data to pianobar's control FIFO.
// Opens with O_WRONLY|O_NONBLOCK to avoid blocking if pianobar isn't reading.
func (s *PandoraStream) writeToFIFO(data string) error {
	if s.fifoPath == "" {
		return fmt.Errorf("pandora: FIFO not initialized")
	}
	f, err := os.OpenFile(s.fifoPath, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("pandora: open FIFO: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(data)
	return err
}

// monitorCurrentSong polls the currentSong file every 2 seconds
// and updates the stream info when it changes.
func (s *PandoraStream) monitorCurrentSong(ctx context.Context, path string) {
	defer s.monWg.Done()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastContent string
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == lastContent || content == "" {
				continue
			}
			lastContent = content
			info := parsePianobarCurrentSong(content)
			s.setInfo(info)
			slog.Debug("pandora: song updated", "track", info.Track, "artist", info.Artist)
			if s.onChange != nil {
				s.onChange(info)
			}
		}
	}
}

// parsePianobarCurrentSong parses the currentSong CSV format:
// title,,,artist,,,album,,,img_url,,,rating,,,station_name
func parsePianobarCurrentSong(line string) models.StreamInfo {
	parts := strings.Split(line, ",,,")
	info := models.StreamInfo{
		Name:  "Pandora",
		State: "playing",
	}
	if len(parts) > 0 {
		info.Track = parts[0]
	}
	if len(parts) > 1 {
		info.Artist = parts[1]
	}
	if len(parts) > 2 {
		info.Album = parts[2]
	}
	if len(parts) > 3 {
		info.ImageURL = parts[3]
	}
	if len(parts) > 5 {
		info.Station = parts[5]
	}
	return info
}
