package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"

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
`

// AirPlayStream plays AirPlay audio via shairport-sync.
// Persistent — shairport-sync must advertise on the network continuously.
type AirPlayStream struct {
	SubprocStream
	name string
}

// NewAirPlayStream creates a new AirPlay stream.
func NewAirPlayStream(name string) *AirPlayStream {
	return &AirPlayStream{name: name}
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

	cfgContent := fmt.Sprintf(shairportConfTemplate, s.name, port, udpBase, device)
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

	return s.activateBase(ctx, vsrc, dir)
}

func (s *AirPlayStream) Deactivate(ctx context.Context) error {
	slog.Info("airplay: deactivating", "name", s.name)
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
