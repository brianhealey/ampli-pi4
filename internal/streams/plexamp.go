package streams

import (
	"context"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// PlexampStream is a stub for Plexamp integration (not yet supported).
type PlexampStream struct {
	name string
}

// NewPlexampStream creates a new (unsupported) Plexamp stream stub.
func NewPlexampStream(name string) *PlexampStream {
	return &PlexampStream{name: name}
}

func (p *PlexampStream) Activate(_ context.Context, _ int, _ string) error {
	return ErrNotSupported
}

func (p *PlexampStream) Deactivate(_ context.Context) error { return nil }
func (p *PlexampStream) Connect(_ context.Context, _ int) error { return nil }
func (p *PlexampStream) Disconnect(_ context.Context) error { return nil }
func (p *PlexampStream) SendCmd(_ context.Context, _ string) error { return nil }

func (p *PlexampStream) Info() models.StreamInfo {
	return models.StreamInfo{
		Name:  p.name,
		State: "stopped",
		Track: "Plexamp not yet supported",
	}
}

func (p *PlexampStream) IsPersistent() bool { return false }
func (p *PlexampStream) Type() string        { return "plexamp" }
