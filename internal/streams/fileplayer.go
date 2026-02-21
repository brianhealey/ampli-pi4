package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// FilePlayerStream plays a local file or directory via VLC.
// Non-persistent — only needed when actively playing.
type FilePlayerStream struct {
	SubprocStream
	name string
	path string
}

// NewFilePlayerStream creates a new file player stream.
func NewFilePlayerStream(name, path string) *FilePlayerStream {
	return &FilePlayerStream{
		name: name,
		path: path,
	}
}

// Activate creates the config dir and starts VLC.
func (s *FilePlayerStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("file_player: activating", "name", s.name, "path", s.path)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("file_player activate: %w", err)
	}

	device := VirtualOutputDevice(vsrc)
	path := s.path

	s.sup = NewSupervisor("file_player/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("vlc"),
			"--intf", "dummy",
			"--aout", "alsa",
			"--alsa-audio-device", device,
			"--no-video",
			path,
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	s.setInfo(models.StreamInfo{Name: s.name, State: "playing"})
	return s.activateBase(ctx, vsrc, dir)
}

func (s *FilePlayerStream) Deactivate(ctx context.Context) error {
	slog.Info("file_player: deactivating", "name", s.name)
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
