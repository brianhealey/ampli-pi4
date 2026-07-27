package config

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

const (
	configFileName  = "house.json"
	debounceDelay   = 500 * time.Millisecond
)

// JSONStore is an atomic JSON file store with debounced writes.
type JSONStore struct {
	mu       sync.Mutex
	path     string
	timer    *time.Timer
	pending  *models.State
	writing  bool // true while writeAtomic is in progress
}

// NewJSONStore creates a new JSON store in the given config directory.
func NewJSONStore(configDir string) *JSONStore {
	return &JSONStore{
		path: filepath.Join(configDir, configFileName),
	}
}

// Path returns the file path used by this store.
func (s *JSONStore) Path() string { return s.path }

// Load reads the state from disk. Returns DefaultState on ENOENT or parse errors.
func (s *JSONStore) Load() (*models.State, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			def := models.DefaultState()
			return &def, nil
		}
		return nil, err
	}

	var state models.State
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Warn("config: corrupt JSON config, using defaults", "path", s.path, "err", err)
		def := models.DefaultState()
		return &def, nil
	}

	migrateState(&state)
	return &state, nil
}

// Save schedules a debounced write of the state to disk.
// The actual write happens after 500ms of no further Save calls.
func (s *JSONStore) Save(state *models.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Take a copy so we don't hold a reference to the caller's state
	stateCopy := *state
	s.pending = &stateCopy

	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(debounceDelay, func() {
		s.doWrite()
	})
	return nil
}

// doWrite executes the debounced write operation.
// Releases the lock during disk I/O to prevent blocking audio operations.
func (s *JSONStore) doWrite() {
	s.mu.Lock()

	if s.pending == nil {
		s.mu.Unlock()
		return
	}

	s.writing = true
	st := s.pending
	s.pending = nil // Clear pending before write
	s.mu.Unlock()   // Release lock before I/O to avoid blocking audio operations

	err := s.writeAtomic(st)

	s.mu.Lock()
	s.writing = false
	s.mu.Unlock()

	if err != nil {
		slog.Error("config: failed to write state", "path", s.path, "err", err)
	}
}

// Flush forces an immediate write of any pending state.
// This blocks until the write is complete.
func (s *JSONStore) Flush() error {
	s.mu.Lock()

	// Cancel any pending debounced write
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	// Nothing to write
	if s.pending == nil {
		s.mu.Unlock()
		return nil
	}

	// Get the pending state and clear it
	st := s.pending
	s.pending = nil
	s.writing = true
	s.mu.Unlock() // Release lock before I/O

	err := s.writeAtomic(st)

	s.mu.Lock()
	s.writing = false
	s.mu.Unlock()

	return err
}

func (s *JSONStore) writeAtomic(state *models.State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	// Write to temp file, then rename (atomic on Linux)
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
