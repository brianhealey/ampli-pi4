package logs

import (
	"fmt"
	"regexp"
	"strings"
)

// Line count bounds for a single log request.
const (
	DefaultLines = 200
	MaxLines     = 5000
)

// MaxFilterLength bounds the substring filter so a pathological query cannot
// make matching expensive.
const MaxFilterLength = 200

// relativeSince matches a relative time window such as "30m", "2h" or "7d".
var relativeSince = regexp.MustCompile(`^([0-9]{1,4})([smhd])$`)

// sinceUnits maps the shorthand suffix to the word journalctl expects.
var sinceUnits = map[string]string{
	"s": "seconds",
	"m": "minutes",
	"h": "hours",
	"d": "days",
}

// Query describes a log read request. Fields are validated by NewQuery before
// any of them reach an external command.
type Query struct {
	Source   string
	Lines    int
	Since    string // journalctl-safe expression, e.g. "2 hours ago"
	Filter   string // case-insensitive substring match, applied in-process
	MinLevel Level
}

// NewQuery validates raw request parameters into a Query.
// Empty values fall back to defaults; invalid values are rejected so a bad
// request never silently returns the wrong logs.
func NewQuery(source, lines, since, filter, level string) (Query, error) {
	q := Query{
		Source:   strings.TrimSpace(source),
		Lines:    DefaultLines,
		Filter:   strings.TrimSpace(filter),
		MinLevel: LevelDebug,
	}

	if lines != "" {
		n, err := parsePositiveInt(lines)
		if err != nil {
			return Query{}, fmt.Errorf("invalid lines: %w", err)
		}
		q.Lines = min(n, MaxLines)
	}

	if since != "" {
		normalized, err := normalizeSince(since)
		if err != nil {
			return Query{}, err
		}
		q.Since = normalized
	}

	if level != "" {
		parsed, ok := ParseLevel(level)
		if !ok {
			return Query{}, fmt.Errorf("invalid level %q", level)
		}
		q.MinLevel = parsed
	}

	if len(q.Filter) > MaxFilterLength {
		return Query{}, fmt.Errorf("filter must be at most %d characters", MaxFilterLength)
	}

	return q, nil
}

// normalizeSince converts a "30m"-style window into the phrasing journalctl
// understands. Only this shape is accepted — arbitrary date strings are
// rejected so nothing unvetted is ever passed as a command argument.
func normalizeSince(since string) (string, error) {
	m := relativeSince.FindStringSubmatch(strings.TrimSpace(since))
	if m == nil {
		return "", fmt.Errorf("invalid since %q: expected a relative window such as 30m, 2h or 7d", since)
	}
	return fmt.Sprintf("%s %s ago", m[1], sinceUnits[m[2]]), nil
}

// parsePositiveInt parses a strictly positive decimal integer.
func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%q is not a number", s)
		}
		n = n*10 + int(r-'0')
		if n > MaxLines {
			return MaxLines, nil
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return n, nil
}

// matches reports whether an entry satisfies the query's level and text filter.
func (q Query) matches(e Entry) bool {
	if !e.Level.AtLeast(q.MinLevel) {
		return false
	}
	if q.Filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(e.Message), strings.ToLower(q.Filter)) ||
		strings.Contains(strings.ToLower(e.Unit), strings.ToLower(q.Filter))
}
