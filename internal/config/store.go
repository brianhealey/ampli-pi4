// Package config handles loading and saving AmpliPi system state.
package config

import "github.com/micro-nova/amplipi-go/internal/models"

// Store is the interface for persisting system state.
type Store interface {
	// Load loads the current state. Returns DefaultState if no file exists.
	Load() (*models.State, error)

	// Save persists the state. Implementations may debounce rapid saves.
	Save(state *models.State) error

	// Path returns the file path used by this store.
	Path() string

	// Flush forces an immediate write of any pending state.
	Flush() error
}
