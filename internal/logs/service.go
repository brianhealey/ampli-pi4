package logs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Source describes a readable log stream.
type Source struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Result is the outcome of a log read.
type Result struct {
	Source    string  `json:"source"`
	Available bool    `json:"available"`
	Reason    string  `json:"reason,omitempty"`
	Entries   []Entry `json:"entries"`
	Count     int     `json:"count"`
}

// ErrUnknownSource is returned when a request names a source that is not
// registered.
var ErrUnknownSource = errors.New("unknown log source")

// reader fetches raw entries for one source.
type reader interface {
	Read(ctx context.Context, q Query, source string) ([]Entry, error)
}

// Service exposes the device's log streams behind a single read method.
type Service struct {
	sources []Source
	readers map[string]reader
}

// NewService creates a Service. buf supplies the in-process application log
// and unitName is the systemd unit the daemon runs as (e.g. "amplipi.service").
func NewService(buf *RingBuffer, unitName string) *Service {
	if buf == nil {
		buf = NewRingBuffer(DefaultBufferSize)
	}
	if unitName == "" {
		unitName = "amplipi.service"
	}

	return &Service{
		sources: []Source{
			{ID: AppSource, Label: "Application", Description: "AmpliPi daemon log captured in memory"},
			{ID: "service", Label: "Service", Description: "systemd journal for " + unitName},
			{ID: "system", Label: "System", Description: "full systemd journal for all units"},
			{ID: "kernel", Label: "Kernel", Description: "kernel ring buffer (dmesg)"},
		},
		readers: map[string]reader{
			AppSource: bufferReader{buf: buf},
			"service": journalReader{journalScope{unit: unitName}},
			"system":  journalReader{journalScope{}},
			"kernel":  journalReader{journalScope{kernel: true}},
		},
	}
}

// Sources lists the readable log streams.
func (s *Service) Sources() []Source {
	out := make([]Source, len(s.sources))
	copy(out, s.sources)
	return out
}

// Read returns the most recent entries for the requested source, filtered by
// the query and ordered oldest-first.
//
// A source that exists but cannot be read (no systemd, insufficient privileges)
// yields a Result with Available=false and an explanatory Reason rather than an
// error — the caller renders that as an empty pane with a note.
func (s *Service) Read(ctx context.Context, q Query) (Result, error) {
	r, ok := s.readers[q.Source]
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownSource, q.Source)
	}

	entries, err := r.Read(ctx, q, q.Source)
	if err != nil {
		return Result{Source: q.Source, Available: false, Reason: err.Error(), Entries: []Entry{}}, nil
	}

	entries = filterEntries(entries, q)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Time.Before(entries[j].Time) })
	entries = tail(entries, q.Lines)

	return Result{
		Source:    q.Source,
		Available: true,
		Entries:   entries,
		Count:     len(entries),
	}, nil
}

// filterEntries keeps entries matching the query's level and text filter.
func filterEntries(entries []Entry, q Query) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if q.matches(e) {
			out = append(out, e)
		}
	}
	return out
}

// tail returns at most n entries from the end of the slice.
func tail(entries []Entry, n int) []Entry {
	if n <= 0 || len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

// bufferReader reads the in-memory application log.
type bufferReader struct {
	buf *RingBuffer
}

// Read returns the buffered entries, applying the query's time window.
func (b bufferReader) Read(_ context.Context, q Query, _ string) ([]Entry, error) {
	entries := b.buf.Snapshot()
	if q.Since == "" {
		return entries, nil
	}

	// The buffer holds absolute timestamps, so the window is applied here
	// rather than delegated the way journalctl's --since is.
	cutoff, ok := sinceCutoff(q.Since)
	if !ok {
		return entries, nil
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Time.After(cutoff) {
			out = append(out, e)
		}
	}
	return out, nil
}

// sinceCutoff converts a normalised "N unit ago" expression back into an
// absolute instant for in-memory filtering.
func sinceCutoff(since string) (time.Time, bool) {
	var n int
	var unit string
	if _, err := fmt.Sscanf(since, "%d %s ago", &n, &unit); err != nil {
		return time.Time{}, false
	}
	var d time.Duration
	switch unit {
	case "seconds":
		d = time.Duration(n) * time.Second
	case "minutes":
		d = time.Duration(n) * time.Minute
	case "hours":
		d = time.Duration(n) * time.Hour
	case "days":
		d = time.Duration(n) * 24 * time.Hour
	default:
		return time.Time{}, false
	}
	return time.Now().Add(-d), true
}
