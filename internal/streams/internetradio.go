package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// InternetRadioStream plays an internet radio stream via VLC.
// It is persistent — VLC keeps the connection alive even when not physically routed.
type InternetRadioStream struct {
	SubprocStream
	url  string
	name string
}

// NewInternetRadioStream creates a new internet radio stream.
func NewInternetRadioStream(name, url string) *InternetRadioStream {
	return &InternetRadioStream{
		url:  url,
		name: name,
	}
}

// Activate creates the config dir and starts VLC.
func (s *InternetRadioStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("internet_radio: activating", "name", s.name, "url", s.url)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("internet_radio activate: %w", err)
	}

	device := VirtualOutputDevice(vsrc)
	url := s.url

	s.sup = NewSupervisor("internet_radio/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("vlc"),
			"--intf", "dummy",
			"--aout", "alsa",
			"--alsa-audio-device", device,
			"--no-video",
			url,
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		return cmd
	})

	s.setInfo(models.StreamInfo{Name: s.name, State: "playing"})
	return s.activateBase(ctx, vsrc, dir)
}

func (s *InternetRadioStream) Deactivate(ctx context.Context) error {
	slog.Info("internet_radio: deactivating", "name", s.name)
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
