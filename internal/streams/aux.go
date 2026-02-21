package streams

import (
	"context"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// AuxStream is a hardware Aux input passthrough (3.5mm jack via CM108B USB audio).
// No subprocess — audio flows directly through ALSA hardware.
type AuxStream struct {
	name string
}

// NewAuxStream creates a new Aux hardware passthrough stream.
func NewAuxStream(name string) *AuxStream {
	return &AuxStream{name: name}
}

func (a *AuxStream) Activate(_ context.Context, _ int, _ string) error { return nil }
func (a *AuxStream) Deactivate(_ context.Context) error                 { return nil }
func (a *AuxStream) Connect(_ context.Context, _ int) error             { return nil }
func (a *AuxStream) Disconnect(_ context.Context) error                 { return nil }
func (a *AuxStream) SendCmd(_ context.Context, _ string) error          { return nil }

func (a *AuxStream) Info() models.StreamInfo {
	return models.StreamInfo{Name: a.name, State: "playing"}
}

func (a *AuxStream) IsPersistent() bool { return false }
func (a *AuxStream) Type() string        { return "aux" }
