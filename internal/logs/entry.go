// Package logs provides unified read access to the log streams useful when
// debugging an AmpliPi device: the daemon's own in-memory log, its systemd
// journal unit, the full system journal, and the kernel ring buffer.
package logs

import (
	"strings"
	"time"
)

// Level is a normalised severity shared by every log source, so entries from
// slog and from syslog priorities can be filtered and rendered uniformly.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// levelRank orders severities for threshold filtering.
var levelRank = map[Level]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
}

// ParseLevel normalises a level name. Unknown values yield ok=false so callers
// can reject a bad filter rather than silently widening it.
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn", "warning":
		return LevelWarn, true
	case "error", "err", "fatal", "crit", "critical":
		return LevelError, true
	default:
		return "", false
	}
}

// AtLeast reports whether l is at or above the given threshold.
func (l Level) AtLeast(threshold Level) bool {
	return levelRank[l] >= levelRank[threshold]
}

// Entry is a single normalised log line.
type Entry struct {
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Message string    `json:"message"`
	Source  string    `json:"source"` // source id the entry was read from
	Unit    string    `json:"unit,omitempty"`
}

// levelFromSyslogPriority maps an RFC 5424 priority (0-7) to a Level.
func levelFromSyslogPriority(priority int) Level {
	switch {
	case priority <= 3: // emerg, alert, crit, err
		return LevelError
	case priority == 4: // warning
		return LevelWarn
	case priority <= 6: // notice, info
		return LevelInfo
	default: // debug
		return LevelDebug
	}
}
