package logs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// journalTimeout bounds a journalctl invocation. Reading thousands of lines
// from a large journal is slow enough to need a ceiling, but a stats page
// should never hang on it.
const journalTimeout = 15 * time.Second

// errJournalUnavailable is returned when journalctl is not installed, which is
// the normal case for local development runs.
var errJournalUnavailable = errors.New("journalctl not available on this system")

// journalReader reads entries from the systemd journal via journalctl.
// scope selects which slice of the journal to read.
type journalReader struct {
	scope journalScope
}

// journalScope is a fixed, non-user-controlled selector for journalctl. Only
// values constructed in this package ever reach the command line.
type journalScope struct {
	unit   string // systemd unit to filter by; empty means the whole journal
	kernel bool   // pass -k to read the kernel ring buffer
}

// Read runs journalctl and returns entries oldest-first.
func (j journalReader) Read(ctx context.Context, q Query, source string) ([]Entry, error) {
	bin, err := exec.LookPath("journalctl")
	if err != nil {
		return nil, errJournalUnavailable
	}

	cmdCtx, cancel := context.WithTimeout(ctx, journalTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, bin, j.args(q)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return parseJournalJSON(out, source), nil
}

// args builds the journalctl argument list. Every value is either a constant
// or a validated field of Query — no user string is passed through verbatim.
func (j journalReader) args(q Query) []string {
	args := []string{"--no-pager", "-o", "json", "-n", strconv.Itoa(q.Lines)}
	if j.scope.kernel {
		args = append(args, "-k")
	}
	if j.scope.unit != "" {
		args = append(args, "-u", j.scope.unit)
	}
	if q.Since != "" {
		args = append(args, "--since", q.Since)
	}
	return args
}

// journalRecord is the subset of journalctl's JSON export format we consume.
// Numeric fields arrive as strings, and MESSAGE is occasionally a byte array
// when the payload is not valid UTF-8.
type journalRecord struct {
	Realtime   string          `json:"__REALTIME_TIMESTAMP"`
	Priority   string          `json:"PRIORITY"`
	Message    json.RawMessage `json:"MESSAGE"`
	Identifier string          `json:"SYSLOG_IDENTIFIER"`
	Unit       string          `json:"_SYSTEMD_UNIT"`
}

// parseJournalJSON converts journalctl's line-delimited JSON into entries.
// Malformed lines are skipped rather than failing the whole read.
func parseJournalJSON(data []byte, source string) []Entry {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)

	var entries []Entry
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec journalRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		entries = append(entries, rec.toEntry(source))
	}
	return entries
}

// toEntry normalises a journal record.
func (r journalRecord) toEntry(source string) Entry {
	e := Entry{
		Message: decodeJournalMessage(r.Message),
		Source:  source,
		Level:   LevelInfo,
		Unit:    r.Unit,
	}
	if e.Unit == "" {
		e.Unit = r.Identifier
	}
	if p, err := strconv.Atoi(r.Priority); err == nil {
		e.Level = levelFromSyslogPriority(p)
	}
	// __REALTIME_TIMESTAMP is microseconds since the Unix epoch.
	if usec, err := strconv.ParseInt(r.Realtime, 10, 64); err == nil {
		e.Time = time.UnixMicro(usec)
	}
	return e
}

// decodeJournalMessage handles both string and byte-array MESSAGE encodings.
func decodeJournalMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var b []byte
	if err := json.Unmarshal(raw, &b); err == nil {
		return string(b)
	}
	return string(raw)
}
