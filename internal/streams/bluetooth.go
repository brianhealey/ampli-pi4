package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"syscall"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// BluetoothStream receives audio from a Bluetooth A2DP source via bluealsa.
// Persistent — must remain discoverable and connected between source switches.
type BluetoothStream struct {
	SubprocStream
	name string
}

// NewBluetoothStream creates a new Bluetooth stream.
func NewBluetoothStream(name string) *BluetoothStream {
	return &BluetoothStream{name: name}
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

	return s.activateBase(ctx, vsrc, dir)
}

func (s *BluetoothStream) Deactivate(ctx context.Context) error {
	slog.Info("bluetooth: deactivating", "name", s.name)
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
