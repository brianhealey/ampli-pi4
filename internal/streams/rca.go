package streams

import (
	"context"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// RCAStream is a hardware RCA input passthrough.
// No subprocess — audio flows directly through the hardware routing matrix.
type RCAStream struct {
	name string
}

// NewRCAStream creates a new RCA hardware passthrough stream.
func NewRCAStream(name string) *RCAStream {
	return &RCAStream{name: name}
}

func (r *RCAStream) Activate(_ context.Context, _ int, _ string) error { return nil }
func (r *RCAStream) Deactivate(_ context.Context) error                 { return nil }
func (r *RCAStream) Connect(_ context.Context, _ int) error             { return nil }
func (r *RCAStream) Disconnect(_ context.Context) error                 { return nil }
func (r *RCAStream) SendCmd(_ context.Context, _ string) error          { return nil }

func (r *RCAStream) Info() models.StreamInfo {
	return models.StreamInfo{Name: r.name, State: "playing"}
}

func (r *RCAStream) IsPersistent() bool { return false }
func (r *RCAStream) Type() string        { return "rca" }
