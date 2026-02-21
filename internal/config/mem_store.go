package config

import (
	"sync"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// MemStore is an in-memory Store for tests that never writes to disk.
type MemStore struct {
	mu    sync.Mutex
	state *models.State
}

// NewMemStore returns a new in-memory store with nil state (defaults to DefaultState on Load).
func NewMemStore() *MemStore {
	return &MemStore{}
}

// Load returns a copy of the stored state, or DefaultState if none has been saved yet.
func (m *MemStore) Load() (*models.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == nil {
		def := models.DefaultState()
		return &def, nil
	}
	cp := m.state.DeepCopy()
	return &cp, nil
}

// Save stores a deep copy of the given state in memory.
func (m *MemStore) Save(state *models.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := state.DeepCopy()
	m.state = &cp
	return nil
}

// Path returns ":memory:" to indicate this is an in-memory store.
func (m *MemStore) Path() string { return ":memory:" }

// Flush is a no-op for in-memory stores.
func (m *MemStore) Flush() error { return nil }

// Ensure MemStore implements config.Store
var _ Store = (*MemStore)(nil)
