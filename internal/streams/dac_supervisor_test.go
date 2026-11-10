package streams

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// withPhysicalOutputs sets the available DAC outputs for one test.
func withPhysicalOutputs(t *testing.T, outputs []int) {
	t.Helper()
	prev := physicalOutputs()
	SetAvailablePhysicalOutputs(outputs)
	t.Cleanup(func() { SetAvailablePhysicalOutputs(prev) })
}

// ─── NewALSALoop ─────────────────────────────────────────────────────────────

func TestNewALSALoop_RefusesUnavailableOutput(t *testing.T) {
	withPhysicalOutputs(t, []int{0})

	for _, phys := range []int{1, 2, 3} {
		loop, err := NewALSALoop(0, phys)
		if err == nil {
			t.Fatalf("ch%d: expected an error on a single-DAC unit, got loop routed to ch%d", phys, loop.physSrc)
		}
		if !errors.Is(err, ErrPhysicalOutputUnavailable) {
			t.Errorf("ch%d: err = %v, want ErrPhysicalOutputUnavailable", phys, err)
		}
		if loop != nil {
			t.Errorf("ch%d: loop should be nil on error", phys)
		}
	}
}

func TestNewALSALoop_AvailableOutput(t *testing.T) {
	withPhysicalOutputs(t, []int{0, 1, 2, 3})

	for _, phys := range []int{0, 1, 2, 3} {
		loop, err := NewALSALoop(4, phys)
		if err != nil {
			t.Fatalf("ch%d: unexpected error: %v", phys, err)
		}
		if loop.physSrc != phys {
			t.Errorf("ch%d: loop routed to ch%d", phys, loop.physSrc)
		}
		if !loop.sup.logStderr {
			t.Errorf("ch%d: alsaloop stderr should be logged", phys)
		}
		if loop.sup.retryForever {
			t.Errorf("ch%d: alsaloop retry policy changed", phys)
		}
	}
}

func TestManager_ConnectToMissingDACReportsUnavailable(t *testing.T) {
	withPhysicalOutputs(t, []int{0})

	var mu sync.Mutex
	infos := map[int]models.StreamInfo{}
	m := NewManager(t.TempDir(), func(id int, info models.StreamInfo) {
		mu.Lock()
		infos[id] = info
		mu.Unlock()
	})
	ctx := context.Background()
	t.Cleanup(func() { _ = m.Shutdown(ctx) })

	streams := []models.Stream{{ID: 1002, Name: "Radio", Type: "internet_radio",
		Config: map[string]interface{}{"url": "http://example.com/stream"}}}
	sources := []models.Source{{ID: 2, Input: "stream=1002"}}
	if err := m.Sync(ctx, streams, sources); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	m.mu.Lock()
	phys := m.streams[1002].PhysSrc
	m.mu.Unlock()
	if phys != -1 {
		t.Errorf("stream connected to physSrc %d, want not connected", phys)
	}
	mu.Lock()
	info := infos[1002]
	mu.Unlock()
	if info.State != "unavailable" {
		t.Errorf("stream state = %q, want unavailable", info.State)
	}
}

// ─── Supervisor retry policy ─────────────────────────────────────────────────

// waitFor polls cond until it is true or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func (s *Supervisor) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func TestSupervisor_DefaultGivesUpAfterMaxFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not available")
	}
	var mu sync.Mutex
	calls := 0
	sup := NewSupervisor("test-giveup", func() *exec.Cmd {
		mu.Lock()
		calls++
		mu.Unlock()
		return exec.Command("false")
	})
	sup.maxFails = 3
	sup.maxBackoff = 5 * time.Millisecond

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 3*time.Second, func() bool { return !sup.isRunning() }) {
		_ = sup.Stop()
		t.Fatal("supervisor without retryForever should give up")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != sup.maxFails {
		t.Errorf("calls = %d, want %d", calls, sup.maxFails)
	}
}

func TestSupervisor_RetryForeverKeepsRetrying(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not available")
	}
	var mu sync.Mutex
	calls := 0
	sup := NewSupervisor("test-retry-forever", func() *exec.Cmd {
		mu.Lock()
		calls++
		mu.Unlock()
		return exec.Command("false")
	})
	sup.maxFails = 2
	sup.maxBackoff = 5 * time.Millisecond
	sup.retryForever = true

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sup.Stop() }()

	target := sup.maxFails * 4
	ok := waitFor(t, 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return calls >= target
	})
	if !ok {
		mu.Lock()
		t.Fatalf("calls = %d, want at least %d", calls, target)
	}
	if !sup.isRunning() {
		t.Error("supervisor with retryForever stopped supervising")
	}
	sup.mu.Lock()
	backoff := sup.backoff
	sup.mu.Unlock()
	if backoff > sup.maxBackoff {
		t.Errorf("backoff %v exceeds cap %v", backoff, sup.maxBackoff)
	}
}

func TestSupervisor_RetryForeverStillGivesUpOnMissingBinary(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	sup := NewSupervisor("test-missing-binary", func() *exec.Cmd {
		mu.Lock()
		calls++
		mu.Unlock()
		return exec.Command("/nonexistent/amplipi-test-binary")
	})
	sup.retryForever = true
	sup.maxBackoff = 5 * time.Millisecond

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 3*time.Second, func() bool { return !sup.isRunning() }) {
		_ = sup.Stop()
		t.Fatal("missing binary should give up even with retryForever")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// ─── Internet radio ──────────────────────────────────────────────────────────

func TestInternetRadio_VLCQuitsOnFailureAndRetriesForever(t *testing.T) {
	s := NewInternetRadioStream("Radio", "http://example.com/stream")
	ctx := context.Background()
	if err := s.Activate(ctx, 0, t.TempDir()); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	sup := s.sup
	defer func() { _ = s.Deactivate(ctx) }()

	args := sup.buildCmd().Args
	if len(args) < 2 || args[len(args)-2] != "http://example.com/stream" || args[len(args)-1] != "vlc://quit" {
		t.Errorf("vlc args = %v, want URL followed by vlc://quit", args)
	}
	if !sup.retryForever {
		t.Error("internet radio supervisor should retry forever")
	}
	if !sup.logStderr {
		t.Error("internet radio supervisor should log stderr")
	}
	if sup.healthCheck == nil || sup.healthGrace != pcmWatchGrace {
		t.Error("internet radio supervisor should watch its audio output")
	}
}

// ─── Child stderr logging ────────────────────────────────────────────────────

type capturedLog struct {
	mu    sync.Mutex
	lines []string
}

func (c *capturedLog) log(msg string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, fmt.Sprint(append([]any{msg}, args...)...))
}

func (c *capturedLog) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

func TestChildLogWriter_SplitsLines(t *testing.T) {
	var c capturedLog
	w := newChildLogWriter("vlc")
	w.log = c.log

	_, _ = w.Write([]byte("first line\nsecond "))
	_, _ = w.Write([]byte("line\r\n\n   \npartial"))
	if got := len(c.all()); got != 2 {
		t.Fatalf("logged %d lines before Close, want 2: %v", got, c.all())
	}
	w.Close()
	lines := c.all()
	if len(lines) != 3 {
		t.Fatalf("logged %d lines, want 3: %v", len(lines), lines)
	}
	for i, want := range []string{"first line", "second line", "partial"} {
		if !strings.Contains(lines[i], want) || !strings.Contains(lines[i], "vlc") {
			t.Errorf("line %d = %q, want it to contain %q and the name", i, lines[i], want)
		}
	}
}

func TestChildLogWriter_RateLimited(t *testing.T) {
	var c capturedLog
	now := time.Unix(1000, 0)
	w := newChildLogWriter("alsaloop")
	w.log = c.log
	w.now = func() time.Time { return now }

	var buf bytes.Buffer
	for i := 0; i < childLogMaxLines+30; i++ {
		fmt.Fprintf(&buf, "xrun %d\n", i)
	}
	_, _ = w.Write(buf.Bytes())
	if got := len(c.all()); got != childLogMaxLines {
		t.Fatalf("logged %d lines in one window, want %d", got, childLogMaxLines)
	}

	// Next window: the suppressed count is reported, then logging resumes.
	now = now.Add(childLogWindow)
	_, _ = w.Write([]byte("after window\n"))
	lines := c.all()
	if len(lines) != childLogMaxLines+2 {
		t.Fatalf("logged %d lines, want %d: %v", len(lines), childLogMaxLines+2, lines[childLogMaxLines:])
	}
	if !strings.Contains(lines[childLogMaxLines], "suppressed") || !strings.Contains(lines[childLogMaxLines], "30") {
		t.Errorf("summary line = %q, want suppressed count 30", lines[childLogMaxLines])
	}
	if !strings.Contains(lines[childLogMaxLines+1], "after window") {
		t.Errorf("last line = %q", lines[childLogMaxLines+1])
	}
}

func TestChildLogWriter_CapsLongLines(t *testing.T) {
	var c capturedLog
	w := newChildLogWriter("vlc")
	w.log = c.log
	_, _ = w.Write(bytes.Repeat([]byte("a"), childLogMaxLineLen*2+10))
	if got := len(c.all()); got != 2 {
		t.Errorf("logged %d chunks for an unterminated long line, want 2", got)
	}
	if len(w.buf) != 10 {
		t.Errorf("buffered %d bytes, want 10", len(w.buf))
	}
}

func TestSupervisor_LogsChildStderr(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	var buf syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	sup := NewSupervisor("internet_radio/Test", func() *exec.Cmd {
		return exec.Command("sh", "-c", "echo 'main input error: cannot open' >&2; exit 1")
	})
	sup.logStderr = true
	sup.maxFails = 1
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 3*time.Second, func() bool { return !sup.isRunning() }) {
		_ = sup.Stop()
		t.Fatal("supervisor did not stop")
	}
	out := buf.String()
	if !strings.Contains(out, "main input error: cannot open") || !strings.Contains(out, "internet_radio/Test") {
		t.Errorf("child stderr not logged with stream name; log:\n%s", out)
	}
}

// syncBuffer is a bytes.Buffer safe for concurrent writes from slog.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
