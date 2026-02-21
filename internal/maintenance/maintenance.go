// Package maintenance provides background system maintenance goroutines for AmpliPi.
// It handles online status checking, release monitoring, and configuration backups.
package maintenance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// dialFunc is a variable so tests can inject a mock dialer.
var dialFunc = func(network, address string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, address, timeout)
}

// Service manages background maintenance goroutines.
type Service struct {
	configDir string
	onOnline  func(bool)   // callback when online status changes
	onRelease func(string) // callback when new release found
}

// New creates a new maintenance Service.
func New(configDir string, onOnline func(bool), onRelease func(string)) *Service {
	return &Service{
		configDir: configDir,
		onOnline:  onOnline,
		onRelease: onRelease,
	}
}

// Start launches all background maintenance goroutines.
// Blocks until ctx is cancelled; all goroutines respect the context.
func (s *Service) Start(ctx context.Context) {
	go s.runCheckOnline(ctx)
	go s.runCheckRelease(ctx)
	go s.runBackup(ctx)

	// Block until cancelled
	<-ctx.Done()
}

// RunBackupNow performs a backup immediately and returns the backup file path or error.
func (s *Service) RunBackupNow() (string, error) {
	return runBackup(s.configDir)
}

// ListBackups returns available backup files sorted by name (newest last).
func ListBackups() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	backupDir := filepath.Join(home, "backups")

	entries, err := os.ReadDir(backupDir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "amplipi-config-") && strings.HasSuffix(e.Name(), ".tar.gz") {
			files = append(files, filepath.Join(backupDir, e.Name()))
		}
	}
	return files, nil
}

// runCheckOnline checks internet connectivity every 5 minutes.
func (s *Service) runCheckOnline(ctx context.Context) {
	lastStatus := false
	first := true

	check := func() {
		conn, err := dialFunc("tcp", "1.1.1.1:53", 3*time.Second)
		online := err == nil
		if conn != nil {
			conn.Close()
		}

		// Write status file
		status := "offline"
		if online {
			status = "online"
		}
		if err2 := os.WriteFile("/tmp/amplipi-online", []byte(status), 0644); err2 != nil {
			slog.Warn("maintenance: failed to write online status", "err", err2)
		}

		// Fire callback if status changed
		if first || online != lastStatus {
			first = false
			lastStatus = online
			if s.onOnline != nil {
				s.onOnline(online)
			}
			slog.Info("maintenance: online status", "online", online)
		}
	}

	check() // immediate first check

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

// releaseResponse is the partial structure of the GitHub releases API response.
type releaseResponse struct {
	TagName string `json:"tag_name"`
}

// runCheckRelease checks for new GitHub releases once at startup and daily at 5am.
func (s *Service) runCheckRelease(ctx context.Context) {
	check := func() {
		version, err := fetchLatestRelease(ctx)
		if err != nil {
			slog.Warn("maintenance: failed to fetch latest release", "err", err)
			return
		}

		if err := os.WriteFile("/tmp/amplipi-latest-release", []byte(version), 0644); err != nil {
			slog.Warn("maintenance: failed to write latest release", "err", err)
		}

		slog.Info("maintenance: latest release", "version", version)
		if s.onRelease != nil {
			s.onRelease(version)
		}
	}

	check() // once at startup

	for {
		now := time.Now()
		// Next 5am
		next5am := time.Date(now.Year(), now.Month(), now.Day(), 5, 0, 0, 0, now.Location())
		if !next5am.After(now) {
			next5am = next5am.Add(24 * time.Hour)
		}
		delay := next5am.Sub(now)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			check()
		}
	}
}

// fetchLatestRelease fetches the latest release tag from GitHub.
func fetchLatestRelease(ctx context.Context) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet,
		"https://api.github.com/repos/micro-nova/AmpliPi/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "AmpliPi/0.5.0-go")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}

	var rel releaseResponse
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", err
	}

	version := strings.TrimPrefix(rel.TagName, "v")
	if version == "" {
		return "", fmt.Errorf("empty tag_name in release response")
	}
	return version, nil
}

// runBackup performs daily backups at 2am.
func (s *Service) runBackup(ctx context.Context) {
	for {
		now := time.Now()
		// Next 2am
		next2am := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
		if !next2am.After(now) {
			next2am = next2am.Add(24 * time.Hour)
		}
		delay := next2am.Sub(now)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
			path, err := runBackup(s.configDir)
			if err != nil {
				slog.Error("maintenance: backup failed", "err", err)
			} else {
				slog.Info("maintenance: backup created", "file", path)
			}
		}
	}
}

// runBackup creates a timestamped backup of the config directory.
func runBackup(configDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}

	backupDir := filepath.Join(home, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// Use configDir if provided, else default to ~/.config/amplipi
	src := configDir
	if src == "" {
		src = filepath.Join(home, ".config", "amplipi")
	}

	date := time.Now().Format("2006-01-02")
	destFile := filepath.Join(backupDir, fmt.Sprintf("amplipi-config-%s.tar.gz", date))

	cmd := exec.Command("tar", "-czf", destFile, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar: %w: %s", err, out)
	}

	// Prune backups older than 90 days
	pruneOldBackups(backupDir, 90*24*time.Hour)

	return destFile, nil
}

// pruneOldBackups deletes backup files older than maxAge from backupDir.
func pruneOldBackups(backupDir string, maxAge time.Duration) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "amplipi-config-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(backupDir, e.Name())
			if err := os.Remove(path); err != nil {
				slog.Warn("maintenance: failed to prune old backup", "file", path, "err", err)
			} else {
				slog.Info("maintenance: pruned old backup", "file", path)
			}
		}
	}
}
