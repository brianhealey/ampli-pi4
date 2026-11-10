package streams

import (
	"bytes"
	"log/slog"
	"regexp"
	"sync"
	"time"
)

// ansiEscape matches terminal escape sequences: CSI (colours, cursor moves),
// OSC (terminated by BEL or ST), charset selection and two-byte ESC
// sequences. Some children (CamillaDSP) colour their log output even when
// stderr is not a terminal, which leaves the journal full of "\x1b[31m" noise.
var ansiEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[()*+][0-9A-Za-z]|[@-Z\\-_])`)

// stripANSI removes terminal escape sequences from b.
func stripANSI(b []byte) []byte {
	if bytes.IndexByte(b, 0x1b) < 0 {
		return b
	}
	return ansiEscape.ReplaceAll(b, nil)
}

// Limits for forwarding a child process's stderr into the daemon log. A chatty
// or looping child must not flood the journal: at most childLogMaxLines lines
// are logged per childLogWindow, the rest are counted and reported as one line.
const (
	childLogMaxLines   = 20
	childLogWindow     = 10 * time.Second
	childLogMaxLineLen = 512
)

// childLogWriter is an io.Writer that splits a child's output into lines and
// logs each one at Info with the supervisor name, rate-limited per window.
// One writer serves one process run; exec copies into it from a single
// goroutine, but Close may race with that copy, so it is locked.
type childLogWriter struct {
	name string
	now  func() time.Time
	log  func(msg string, args ...any)

	mu          sync.Mutex
	buf         []byte
	windowStart time.Time
	inWindow    int
	suppressed  int
}

func newChildLogWriter(name string) *childLogWriter {
	return &childLogWriter{name: name, now: time.Now, log: slog.Info}
}

// Write implements io.Writer. It never fails, so the child is never blocked or
// killed by a logging problem.
func (w *childLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	// An overlong line without a newline is logged in pieces.
	for len(w.buf) >= childLogMaxLineLen {
		w.emit(w.buf[:childLogMaxLineLen])
		w.buf = w.buf[childLogMaxLineLen:]
	}
	return len(p), nil
}

// Close logs any trailing partial line and the count of suppressed lines.
func (w *childLogWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
	w.flushSuppressed()
}

// emit logs one line, subject to the rate limit. Caller holds w.mu.
func (w *childLogWriter) emit(line []byte) {
	line = bytes.TrimRight(line, "\r")
	line = bytes.TrimSpace(stripANSI(line))
	if len(line) == 0 {
		return
	}
	now := w.now()
	if w.windowStart.IsZero() || now.Sub(w.windowStart) >= childLogWindow {
		w.flushSuppressed()
		w.windowStart = now
		w.inWindow = 0
	}
	if w.inWindow >= childLogMaxLines {
		w.suppressed++
		return
	}
	w.inWindow++
	w.log("supervisor: child stderr", "name", w.name, "line", string(line))
}

// flushSuppressed reports lines dropped in the current window. Caller holds w.mu.
func (w *childLogWriter) flushSuppressed() {
	if w.suppressed == 0 {
		return
	}
	w.log("supervisor: child stderr lines suppressed", "name", w.name, "count", w.suppressed)
	w.suppressed = 0
}
