package logs

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRingBufferWrapsAndKeepsOrder(t *testing.T) {
	b := NewRingBuffer(3)
	for _, msg := range []string{"a", "b", "c", "d", "e"} {
		b.Append(Entry{Message: msg})
	}

	if b.Len() != 3 {
		t.Fatalf("len = %d, want 3", b.Len())
	}

	got := b.Snapshot()
	want := []string{"c", "d", "e"} // oldest two evicted, chronological order kept
	for i := range want {
		if got[i].Message != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i].Message, want[i])
		}
	}
}

func TestRingBufferPartiallyFilled(t *testing.T) {
	b := NewRingBuffer(10)
	b.Append(Entry{Message: "only"})

	got := b.Snapshot()
	if len(got) != 1 || got[0].Message != "only" {
		t.Errorf("snapshot = %+v, want one entry", got)
	}
}

func TestHandlerCapturesRecordsWithAttrs(t *testing.T) {
	buf := NewRingBuffer(10)
	logger := slog.New(NewHandler(slog.NewTextHandler(discard{}, nil), buf))

	logger.With("unit", 2).Warn("i2c timeout", "reg", "0x08")

	entries := buf.Snapshot()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want 1", len(entries))
	}
	if entries[0].Level != LevelWarn {
		t.Errorf("level = %q, want warn", entries[0].Level)
	}
	// Both the pre-bound and the call-site attribute must be rendered.
	for _, want := range []string{"i2c timeout", "unit=2", "reg=0x08"} {
		if !strings.Contains(entries[0].Message, want) {
			t.Errorf("message %q missing %q", entries[0].Message, want)
		}
	}
}

func TestHandlerSiblingsDoNotShareAttrs(t *testing.T) {
	buf := NewRingBuffer(10)
	base := slog.New(NewHandler(slog.NewTextHandler(discard{}, nil), buf))

	base.With("a", 1).Info("first")
	base.With("b", 2).Info("second")

	entries := buf.Snapshot()
	if strings.Contains(entries[1].Message, "a=1") {
		t.Errorf("second entry leaked a sibling's attribute: %q", entries[1].Message)
	}
}

func TestParseJournalJSON(t *testing.T) {
	raw := []byte(`{"__REALTIME_TIMESTAMP":"1700000000000000","PRIORITY":"3","MESSAGE":"disk failure","_SYSTEMD_UNIT":"amplipi.service"}
not json — skipped
{"__REALTIME_TIMESTAMP":"1700000001000000","PRIORITY":"6","MESSAGE":[104,105],"SYSLOG_IDENTIFIER":"kernel"}
`)

	entries := parseJournalJSON(raw, "system")
	if len(entries) != 2 {
		t.Fatalf("parsed %d entries, want 2 (malformed line skipped)", len(entries))
	}

	if entries[0].Level != LevelError || entries[0].Message != "disk failure" {
		t.Errorf("entry 0 = %+v, want error/disk failure", entries[0])
	}
	if entries[0].Unit != "amplipi.service" {
		t.Errorf("unit = %q, want amplipi.service", entries[0].Unit)
	}
	if !entries[0].Time.Equal(time.UnixMicro(1700000000000000)) {
		t.Errorf("time = %v, want microsecond-precision epoch", entries[0].Time)
	}

	// A non-UTF-8 MESSAGE arrives as a byte array.
	if entries[1].Message != "hi" || entries[1].Level != LevelInfo {
		t.Errorf("entry 1 = %+v, want info/hi", entries[1])
	}
	if entries[1].Unit != "kernel" {
		t.Errorf("unit = %q, want the syslog identifier as fallback", entries[1].Unit)
	}
}

func TestNewQueryValidation(t *testing.T) {
	tests := []struct {
		name      string
		since     string
		lines     string
		level     string
		wantErr   bool
		wantSince string
	}{
		{name: "defaults", wantSince: ""},
		{name: "relative window", since: "2h", wantSince: "2 hours ago"},
		{name: "minutes", since: "30m", wantSince: "30 minutes ago"},
		{name: "shell metacharacters rejected", since: "1h; rm -rf /", wantErr: true},
		{name: "journalctl flag rejected", since: "--output=cat", wantErr: true},
		{name: "free-form date rejected", since: "yesterday", wantErr: true},
		{name: "non-numeric lines rejected", lines: "abc", wantErr: true},
		{name: "unknown level rejected", level: "verbose", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := NewQuery("app", tc.lines, tc.since, "", tc.level)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got query %+v", q)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Since != tc.wantSince {
				t.Errorf("since = %q, want %q", q.Since, tc.wantSince)
			}
		})
	}
}

func TestNewQueryClampsLines(t *testing.T) {
	q, err := NewQuery("app", "999999", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Lines != MaxLines {
		t.Errorf("lines = %d, want clamped to %d", q.Lines, MaxLines)
	}
}

func TestQueryMatches(t *testing.T) {
	q, err := NewQuery("app", "", "", "TIMEOUT", "warn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		entry Entry
		want  bool
	}{
		{Entry{Level: LevelError, Message: "i2c timeout on unit 2"}, true},     // case-insensitive
		{Entry{Level: LevelInfo, Message: "i2c timeout on unit 2"}, false},     // below level
		{Entry{Level: LevelError, Message: "all good"}, false},                 // filter misses
		{Entry{Level: LevelWarn, Message: "x", Unit: "timeout.service"}, true}, // unit matches
	}

	for _, tc := range cases {
		if got := q.matches(tc.entry); got != tc.want {
			t.Errorf("matches(%+v) = %v, want %v", tc.entry, got, tc.want)
		}
	}
}

func TestServiceReadUnknownSource(t *testing.T) {
	svc := NewService(NewRingBuffer(10), "amplipi.service")

	if _, err := svc.Read(t.Context(), Query{Source: "../../etc/passwd"}); err == nil {
		t.Error("expected an error for an unknown source")
	}
}

func TestServiceReadAppSourceSortsAndTails(t *testing.T) {
	buf := NewRingBuffer(10)
	base := time.Now()
	// Appended out of order to prove the service sorts chronologically.
	buf.Append(Entry{Time: base.Add(2 * time.Second), Level: LevelInfo, Message: "third"})
	buf.Append(Entry{Time: base, Level: LevelInfo, Message: "first"})
	buf.Append(Entry{Time: base.Add(time.Second), Level: LevelInfo, Message: "second"})

	svc := NewService(buf, "amplipi.service")
	result, err := svc.Read(t.Context(), Query{Source: AppSource, Lines: 2, MinLevel: LevelDebug})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Available {
		t.Fatalf("app source should always be available, got reason %q", result.Reason)
	}
	// Tail keeps the newest two, still oldest-first.
	if len(result.Entries) != 2 ||
		result.Entries[0].Message != "second" ||
		result.Entries[1].Message != "third" {
		t.Errorf("entries = %+v, want [second third]", result.Entries)
	}
}

func TestLevelAtLeast(t *testing.T) {
	if LevelInfo.AtLeast(LevelWarn) {
		t.Error("info should not satisfy a warn threshold")
	}
	if !LevelError.AtLeast(LevelWarn) {
		t.Error("error should satisfy a warn threshold")
	}
}

// discard is a no-op io.Writer for the inner slog handler under test.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
