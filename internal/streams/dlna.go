package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"

	"github.com/google/uuid"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// DLNAStream is a DLNA/UPnP audio renderer using gmrender-resurrect.
// Persistent — must advertise on the network continuously.
type DLNAStream struct {
	SubprocStream
	name string

	monCancel context.CancelFunc
	monWg     sync.WaitGroup

	onChange func(info models.StreamInfo)
}

// NewDLNAStream creates a new DLNA stream.
func NewDLNAStream(name string, onChange func(models.StreamInfo)) *DLNAStream {
	return &DLNAStream{
		name:     name,
		onChange: onChange,
	}
}

// Activate starts gmrender-resurrect with a per-instance UUID.
func (s *DLNAStream) Activate(ctx context.Context, vsrc int, configDir string) error {
	slog.Info("dlna: activating", "name", s.name)

	dir, err := buildConfigDir(configDir, vsrc)
	if err != nil {
		return fmt.Errorf("dlna activate: %w", err)
	}

	// Generate a stable UUID from the stream name + vsrc for identity persistence
	deviceUUID := uuid.NewString()
	name := s.name
	device := VirtualOutputDevice(vsrc)

	s.sup = NewSupervisor("dlna/"+s.name, func() *exec.Cmd {
		cmd := exec.Command(findBinary("gmrender-resurrect"),
			"-u", deviceUUID,
			"-f", name,
			"--gstout-audiosink=alsasink",
			"--gstout-audiodevice="+device,
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

func (s *DLNAStream) Deactivate(ctx context.Context) error {
	slog.Info("dlna: deactivating", "name", s.name)
	if s.monCancel != nil {
		s.monCancel()
	}
	s.monWg.Wait()
	return s.deactivateBase(ctx)
}

func (s *DLNAStream) Connect(ctx context.Context, physSrc int) error {
	return s.connectBase(ctx, physSrc)
}

func (s *DLNAStream) Disconnect(ctx context.Context) error {
	return s.disconnectBase(ctx)
}

// SendCmd handles DLNA playback controls.
// Metadata and command relay are not implemented in v1.
func (s *DLNAStream) SendCmd(_ context.Context, cmd string) error {
	slog.Debug("dlna: command (not implemented in v1)", "name", s.name, "cmd", cmd)
	return nil
}

func (s *DLNAStream) Info() models.StreamInfo {
	return s.getInfo()
}

func (s *DLNAStream) IsPersistent() bool { return true }
func (s *DLNAStream) Type() string        { return "dlna" }
