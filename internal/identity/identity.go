// Package identity provides system identity information for AmpliPi.
package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DefaultVersion is the fallback version string when metadata.json is not found.
const DefaultVersion = "0.5.0-go"

// Info holds system identity information.
type Info struct {
	Hostname string
	Serial   string // from EEPROM BoardInfo.Serial, or "None" if unreadable
	Version  string // software version string e.g. "0.4.10"
	IsUpdate bool   // true if running from update staging area
	Offline  bool   // populated by maintenance package
}

// GetHostname returns the system hostname.
func GetHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "amplipi"
	}
	return h
}

// GetVersion reads the version from ~/.config/amplipi/metadata.json.
// Falls back to DefaultVersion if the file is missing or unreadable.
func GetVersion() string {
	return GetVersionFromDir("")
}

// GetVersionFromDir reads the version from a specific config directory.
// If dir is empty, uses the default ~/.config/amplipi path.
// This variant is exported for testing.
func GetVersionFromDir(dir string) string {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return DefaultVersion
		}
		dir = filepath.Join(home, ".config", "amplipi")
	}

	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return DefaultVersion
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return DefaultVersion
	}

	if v, ok := meta["version"].(string); ok && v != "" {
		return v
	}
	return DefaultVersion
}

// IsUpdateMode returns true if the process is running from an update staging area.
// This is detected by:
//   - The process executable path containing "amplipi-update"
//   - The presence of /tmp/amplipi-update.flag
func IsUpdateMode() bool {
	exe, err := os.Executable()
	if err == nil && strings.Contains(exe, "amplipi-update") {
		return true
	}
	if _, err := os.Stat("/tmp/amplipi-update.flag"); err == nil {
		return true
	}
	return false
}

// GetOnlineStatus returns true if the system is online.
// Checks (in order):
//  1. /tmp/amplipi-online — written by Go maintenance goroutine
//  2. /home/pi/amplipi-dev/config/online — written by Python backend
//
// Returns false (offline) if neither file exists.
func GetOnlineStatus() bool {
	// Go-written status file
	if data, err := os.ReadFile("/tmp/amplipi-online"); err == nil {
		return strings.TrimSpace(string(data)) == "online"
	}
	// Python-written status file
	if _, err := os.Stat("/home/pi/amplipi-dev/config/online"); err == nil {
		return true
	}
	return false
}
