// Package auth implements cookie/API-key authentication compatible with the
// Python AmpliPi auth system.
package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

const usersFileName = "users.json"

// User represents a single user in the users.json file.
type User struct {
	Type             string `json:"type"`
	AccessKey        string `json:"access_key"`
	AccessKeyUpdated string `json:"access_key_updated"`
	PasswordHash     string `json:"password_hash,omitempty"`
}

// Service handles authentication for AmpliPi.
type Service struct {
	mu        sync.RWMutex
	configDir string
	users     map[string]User
	watcher   *fsnotify.Watcher
}

// NewService creates a new auth service watching the given config directory.
func NewService(configDir string) (*Service, error) {
	s := &Service{
		configDir: configDir,
		users:     make(map[string]User),
	}

	// Load initial state (missing file is OK — open mode)
	if err := s.Reload(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// Watch for changes
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("auth: could not create fsnotify watcher", "err", err)
		return s, nil
	}
	s.watcher = watcher

	usersPath := s.usersPath()
	if err := watcher.Add(filepath.Dir(usersPath)); err != nil {
		slog.Warn("auth: could not watch config dir", "err", err)
	}

	go s.watchLoop(usersPath)
	return s, nil
}

func (s *Service) usersPath() string {
	return filepath.Join(s.configDir, usersFileName)
}

// Reload re-reads the users.json file.
func (s *Service) Reload() error {
	data, err := os.ReadFile(s.usersPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.mu.Lock()
			s.users = make(map[string]User)
			s.mu.Unlock()
			return nil
		}
		return err
	}

	var users map[string]User
	if err := json.Unmarshal(data, &users); err != nil {
		return err
	}

	s.mu.Lock()
	s.users = users
	s.mu.Unlock()
	slog.Debug("auth: reloaded users", "count", len(users))
	return nil
}

// IsOpenMode returns true if no users have a password hash set.
// In open mode, all requests are allowed without authentication.
func (s *Service) IsOpenMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.PasswordHash != "" {
			return false
		}
	}
	return true
}

// VerifyKey returns true if the given access key matches any user's access key.
// Uses constant-time comparison to prevent timing attacks.
func (s *Service) VerifyKey(key string) bool {
	if key == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if subtle.ConstantTimeCompare([]byte(key), []byte(u.AccessKey)) == 1 {
			return true
		}
	}
	return false
}

// Close stops the file watcher.
func (s *Service) Close() {
	if s.watcher != nil {
		s.watcher.Close()
	}
}

func (s *Service) watchLoop(usersPath string) {
	if s.watcher == nil {
		return
	}
	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if event.Name == usersPath && (event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
				if err := s.Reload(); err != nil {
					slog.Warn("auth: failed to reload users", "err", err)
				}
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("auth: watcher error", "err", err)
		}
	}
}
