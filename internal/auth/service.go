// Package auth implements cookie/API-key authentication compatible with the
// Python AmpliPi auth system.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/crypto/argon2"
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

// Argon2 parameters matching Python AmpliPi implementation
const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64MB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// VerifyPassword verifies a password against a stored argon2 hash.
// Returns the user's access key if successful, empty string otherwise.
// Hash format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
func (s *Service) VerifyPassword(password string) (accessKey string, ok bool) {
	if password == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if u.PasswordHash == "" {
			continue
		}
		if verifyArgon2Hash(password, u.PasswordHash) {
			return u.AccessKey, true
		}
	}
	return "", false
}

// verifyArgon2Hash verifies a password against an argon2id hash string.
// Hash format: $argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
func verifyArgon2Hash(password, hashStr string) bool {
	// Parse hash string (format from Python passlib/argon2)
	// Example: $argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$aGFzaGVkdmFsdWU
	parts := splitArgon2Hash(hashStr)
	if parts == nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts.salt)
	if err != nil {
		return false
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts.hash)
	if err != nil {
		return false
	}

	// Compute hash with same parameters
	computedHash := argon2.IDKey([]byte(password), salt, parts.time, parts.memory, parts.threads, uint32(len(expectedHash)))

	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}

type argon2Parts struct {
	time    uint32
	memory  uint32
	threads uint8
	salt    string
	hash    string
}

// splitArgon2Hash parses an argon2id hash string into its components.
func splitArgon2Hash(hashStr string) *argon2Parts {
	// Expected format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	if len(hashStr) < 10 || hashStr[0] != '$' {
		return nil
	}

	// Split by $
	var parts []string
	start := 1
	for i := 1; i < len(hashStr); i++ {
		if hashStr[i] == '$' {
			parts = append(parts, hashStr[start:i])
			start = i + 1
		}
	}
	parts = append(parts, hashStr[start:])

	if len(parts) != 5 {
		return nil
	}

	// parts[0] = "argon2id"
	// parts[1] = "v=19"
	// parts[2] = "m=65536,t=3,p=4"
	// parts[3] = salt (base64)
	// parts[4] = hash (base64)

	if parts[0] != "argon2id" {
		return nil
	}

	// Parse parameters from parts[2]
	var time, memory uint32
	var threads uint8
	params := parts[2]
	for _, param := range splitParams(params) {
		if len(param) < 2 {
			continue
		}
		switch param[0] {
		case 'm':
			if val := parseUint(param[2:]); val > 0 {
				memory = uint32(val)
			}
		case 't':
			if val := parseUint(param[2:]); val > 0 {
				time = uint32(val)
			}
		case 'p':
			if val := parseUint(param[2:]); val > 0 {
				threads = uint8(val)
			}
		}
	}

	if time == 0 || memory == 0 || threads == 0 {
		return nil
	}

	return &argon2Parts{
		time:    time,
		memory:  memory,
		threads: threads,
		salt:    parts[3],
		hash:    parts[4],
	}
}

func splitParams(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func parseUint(s string) uint64 {
	var val uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		val = val*10 + uint64(c-'0')
	}
	return val
}

// GenerateSessionToken creates a cryptographically secure session token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
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
