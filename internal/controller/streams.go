package controller

import (
	"context"
	"fmt"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// GetStreams returns all streams.
func (c *Controller) GetStreams() []models.Stream {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]models.Stream, len(c.state.Streams))
	copy(result, c.state.Streams)
	return result
}

// GetStream returns a single stream by ID.
func (c *Controller) GetStream(id int) (*models.Stream, *models.AppError) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := findStream(&c.state, id)
	if s == nil {
		return nil, models.ErrNotFound("stream not found")
	}
	cp := *s
	return &cp, nil
}

// CreateStream creates a new stream and returns the updated state.
func (c *Controller) CreateStream(_ context.Context, req models.StreamCreate) (models.State, *models.AppError) {
	if req.Name == "" {
		return models.State{}, models.ErrBadRequest("stream name is required")
	}
	if req.Type == "" {
		return models.State{}, models.ErrBadRequest("stream type is required")
	}

	// Reject stream types whose binary isn't installed on this hardware
	if c.profile != nil && !c.profile.StreamAvailable(req.Type) {
		return models.State{}, models.ErrBadRequest(
			fmt.Sprintf("stream type %q is not available on this hardware", req.Type))
	}

	state, err := c.apply(func(s *models.State) error {
		f := false
		stream := models.Stream{
			ID:        nextStreamID(s),
			Name:      req.Name,
			Type:      req.Type,
			Config:    req.Config,
			Disabled:  &f,
			Browsable: &f,
		}
		s.Streams = append(s.Streams, stream)
		return nil
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// SetStream updates a stream by ID.
func (c *Controller) SetStream(_ context.Context, id int, upd models.StreamUpdate) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		stream := findStream(s, id)
		if stream == nil {
			return models.ErrNotFound("stream not found")
		}
		if upd.Name != nil {
			stream.Name = *upd.Name
		}
		if upd.Config != nil {
			if stream.Config == nil {
				stream.Config = make(map[string]interface{})
			}
			for k, v := range upd.Config {
				stream.Config[k] = v
			}
		}
		return nil
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// DeleteStream removes a stream by ID.
func (c *Controller) DeleteStream(_ context.Context, id int) (models.State, *models.AppError) {
	state, err := c.apply(func(s *models.State) error {
		for i, st := range s.Streams {
			if st.ID == id {
				s.Streams = append(s.Streams[:i], s.Streams[i+1:]...)
				return nil
			}
		}
		return models.ErrNotFound(fmt.Sprintf("stream %d not found", id))
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}

// ExecStreamCommand executes a command on a stream (play, pause, next, etc.)
// When a stream Manager is available, routes the command to the stream subprocess
// and returns the current state (stream info is updated asynchronously via
// UpdateStreamInfo callbacks from the subprocess).
// When no Manager is available (nil, used in tests/mock mode), falls back to
// direct state mutation for the standard play/pause/stop commands.
func (c *Controller) ExecStreamCommand(ctx context.Context, id int, cmd string) (models.State, *models.AppError) {
	// Validate that the stream exists first
	c.mu.RLock()
	stream := findStream(&c.state, id)
	c.mu.RUnlock()
	if stream == nil {
		return models.State{}, models.ErrNotFound(fmt.Sprintf("stream %d not found", id))
	}

	// Route to stream manager if available
	if c.streams != nil {
		if err := c.streams.SendCmd(ctx, id, cmd); err != nil {
			return models.State{}, models.ErrInternal(fmt.Sprintf("stream command failed: %v", err))
		}
		// State is updated asynchronously via UpdateStreamInfo; return current snapshot.
		c.mu.RLock()
		state := c.state.DeepCopy()
		c.mu.RUnlock()
		return state, nil
	}

	// Fallback: no Manager configured — update state directly.
	// Handles play/pause/stop in tests and mock/standalone mode.
	state, err := c.apply(func(s *models.State) error {
		st := findStream(s, id)
		if st == nil {
			return models.ErrNotFound(fmt.Sprintf("stream %d not found", id))
		}
		switch cmd {
		case "play":
			st.Info.State = "playing"
		case "pause":
			st.Info.State = "paused"
		case "stop":
			st.Info.State = "stopped"
		default:
			// Accept but ignore unknown commands in fallback mode
		}
		return nil
	})
	if err != nil {
		if appErr, ok := err.(*models.AppError); ok {
			return models.State{}, appErr
		}
		return models.State{}, models.ErrInternal(err.Error())
	}
	return state, nil
}
