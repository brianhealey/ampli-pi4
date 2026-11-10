package streams

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// directRT is a launcher that never uses chrt.
func directRT() *rtLauncher {
	return &rtLauncher{findChrt: func() string { return "" }, prio: camillaRTPriority}
}

// ─── chrt launcher ───────────────────────────────────────────────────────────

func TestRTLauncher_CommandLine(t *testing.T) {
	args := camillaArgs("3805", "/cfg/v5/camilladsp.yml")

	probes := 0
	withChrt := &rtLauncher{
		findChrt: func() string { return "/usr/bin/chrt" },
		probe:    func(string, int) error { probes++; return nil },
		prio:     camillaRTPriority,
	}
	cmd := withChrt.command("/usr/local/bin/camilladsp", args...)
	want := []string{"/usr/bin/chrt", "--rr", "40", "/usr/local/bin/camilladsp",
		"-l", "warn", "-a", "127.0.0.1", "-p", "3805", "/cfg/v5/camilladsp.yml"}
	if cmd.Path != "/usr/bin/chrt" || !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("with chrt: %q %q\nwant %q", cmd.Path, cmd.Args, want)
	}

	noChrt := directRT()
	cmd = noChrt.command("/usr/local/bin/camilladsp", args...)
	want = append([]string{"/usr/local/bin/camilladsp"}, args...)
	if cmd.Path != "/usr/local/bin/camilladsp" || !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("without chrt: %q %q", cmd.Path, cmd.Args)
	}

	// Not permitted (no RLIMIT_RTPRIO): launch directly, probing only once.
	denied := &rtLauncher{
		findChrt: func() string { return "/usr/bin/chrt" },
		probe: func(string, int) error {
			probes++
			return errors.New("chrt: failed to set pid 0's policy: Operation not permitted")
		},
		prio: camillaRTPriority,
	}
	probes = 0
	for i := 0; i < 3; i++ {
		cmd = denied.command("/usr/local/bin/camilladsp", args...)
		if !reflect.DeepEqual(cmd.Args, want) {
			t.Errorf("after a failed probe: %q, want direct launch", cmd.Args)
		}
	}
	if probes != 1 {
		t.Errorf("probed %d times, want once", probes)
	}
}

// fakeChrt writes a chrt stand-in: it either refuses like chrt without
// RLIMIT_RTPRIO (exit 1 before exec) or drops "--rr <prio>" and execs the
// command, as chrt does.
func fakeChrt(t *testing.T, permitted bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "chrt")
	script := "#!/bin/sh\necho \"chrt: failed to set pid 0's policy: Operation not permitted\" >&2\nexit 1\n"
	if permitted {
		script = "#!/bin/sh\nshift 2\nexec \"$@\"\n"
	}
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProbeChrt(t *testing.T) {
	if err := probeChrt(fakeChrt(t, true), 40); err != nil {
		t.Errorf("permitted chrt: %v", err)
	}
	err := probeChrt(fakeChrt(t, false), 40)
	if err == nil || !strings.Contains(err.Error(), "Operation not permitted") {
		t.Errorf("denied chrt: err = %v", err)
	}
}

// With chrt present but not permitted, CamillaDSP still starts (directly),
// and the bridge does not fall back.
func TestEQBridge_StartsWithoutRTWhenChrtDenied(t *testing.T) {
	captureAmixer(t)
	bin, starts := fakeCamillaBinary(t)
	chrt := fakeChrt(t, false)
	f := newFakeCamilla(t, stateReply("Running"))
	fb := &fakeBridge{}
	b := newEQBridge(4, 0, bin, t.TempDir(), *sampleEQForStreams(), fb, nil)
	b.rt = &rtLauncher{findChrt: func() string { return chrt }, probe: probeChrt, prio: camillaRTPriority}
	b.addr = f.addr()
	b.tune = fastSupervisor
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()
	if !waitFor(t, 5*time.Second, func() bool { return starts() == 1 }) {
		t.Fatal("CamillaDSP did not start without chrt")
	}
	time.Sleep(200 * time.Millisecond)
	if s, _ := fb.counts(); s != 0 || starts() != 1 {
		t.Errorf("fallback started %d times, CamillaDSP started %d times", s, starts())
	}
}

// Through a permitted chrt, the supervised PID is CamillaDSP itself (chrt
// execs it), so the process-group kill on Stop reaches it.
func TestEQBridge_ChrtExecsCamilla(t *testing.T) {
	captureAmixer(t)
	bin, starts := fakeCamillaBinary(t)
	chrt := fakeChrt(t, true)
	f := newFakeCamilla(t, stateReply("Running"))
	b := newEQBridge(4, 0, bin, t.TempDir(), *sampleEQForStreams(), &fakeBridge{}, nil)
	b.rt = &rtLauncher{findChrt: func() string { return chrt }, probe: probeChrt, prio: camillaRTPriority}
	b.addr = f.addr()
	b.tune = fastSupervisor
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 5*time.Second, func() bool { return starts() == 1 && b.sup.Pid() > 0 }) {
		_ = b.Stop()
		t.Fatal("not started")
	}
	pid := b.sup.Pid()
	out, _ := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if got := strings.TrimSpace(string(out)); !strings.HasSuffix(got, "sleep") {
		t.Errorf("supervised pid %d runs %q, want the exec'd target", pid, got)
	}
	_ = b.Stop()
	if !waitFor(t, 5*time.Second, func() bool { return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() != nil }) {
		t.Errorf("pid %d still alive after Stop", pid)
	}
}

// ─── Supervisor give-up ordering ─────────────────────────────────────────────

func TestSupervisor_GivesUpWithoutSleepingAfterFinalFailure(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not available")
	}
	var mu sync.Mutex
	var sleeps []time.Duration
	gaveUp := make(chan time.Time, 1)
	sup := NewSupervisor("test-giveup-order", func() *exec.Cmd { return exec.Command("false") })
	sup.maxFails = 2
	sup.initialBackoff = 300 * time.Millisecond
	sup.maxBackoff = 10 * time.Second
	sup.sleepHook = func(d time.Duration) { mu.Lock(); sleeps = append(sleeps, d); mu.Unlock() }
	sup.onGiveUp = func(string) { gaveUp <- time.Now() }

	start := time.Now()
	_ = sup.Start(context.Background())
	var at time.Time
	select {
	case at = <-gaveUp:
	case <-time.After(5 * time.Second):
		_ = sup.Stop()
		t.Fatal("never gave up")
	}
	mu.Lock()
	defer mu.Unlock()
	// One backoff between the two attempts (600 ms), none after the last.
	if len(sleeps) != 1 || sleeps[0] != 600*time.Millisecond {
		t.Errorf("backoff sleeps = %v, want [600ms]", sleeps)
	}
	if el := at.Sub(start); el > 1500*time.Millisecond {
		t.Errorf("gave up after %v; a trailing 1.2 s backoff was slept", el)
	}
}

// The radio's retryForever path still sleeps and retries past maxFails.
func TestSupervisor_RetryForeverStillSleepsPastMaxFails(t *testing.T) {
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("false not available")
	}
	var mu sync.Mutex
	sleeps := 0
	sup := NewSupervisor("test-retry-order", func() *exec.Cmd { return exec.Command("false") })
	sup.maxFails = 2
	sup.retryForever = true
	sup.initialBackoff = time.Millisecond
	sup.maxBackoff = 2 * time.Millisecond
	sup.sleepHook = func(time.Duration) { mu.Lock(); sleeps++; mu.Unlock() }
	gaveUp := false
	sup.onGiveUp = func(string) { gaveUp = true }
	_ = sup.Start(context.Background())
	ok := waitFor(t, 5*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return sleeps >= 5 })
	_ = sup.Stop()
	if !ok || gaveUp {
		t.Errorf("retryForever: sleeps=%d gaveUp=%v", sleeps, gaveUp)
	}
}

// ─── Fallback timing and logging ─────────────────────────────────────────────

// recordHandler keeps every log record about one bridge: a name attribute
// containing "v<vsrc>->" or a vsrc attribute equal to vsrc.
type recordHandler struct {
	match string
	mu    sync.Mutex
	recs  []slog.Record
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	hit := false
	r.Attrs(func(a slog.Attr) bool {
		if (a.Key == "vsrc" && a.Value.String() == h.match) ||
			strings.Contains(a.Value.String(), "v"+h.match+"->") {
			hit = true
		}
		return true
	})
	if hit {
		h.mu.Lock()
		h.recs = append(h.recs, r.Clone())
		h.mu.Unlock()
	}
	return nil
}
func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordHandler) errors() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, r := range h.recs {
		if r.Level >= slog.LevelError {
			out = append(out, r.Message)
		}
	}
	return out
}

// captureLogs routes slog to a handler that keeps records about vsrc match.
func captureLogs(t *testing.T, match string) *recordHandler {
	t.Helper()
	h := &recordHandler{match: match}
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(old) })
	return h
}

// A CamillaDSP that stays up while its websocket port is held by something
// that accepts but never answers: with the production tuning, alsaloop must
// take over in well under 10 s (calculated worst case ~5.7 s), with a single
// ERROR for the event.
func TestEQBridge_WorstCaseFallbackTimeAndSingleError(t *testing.T) {
	captureAmixer(t)
	logs := captureLogs(t, "9")
	bin, starts := fakeCamillaBinary(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { // occupy the port: accept, never answer
		var held []net.Conn
		defer func() {
			for _, c := range held {
				c.Close()
			}
		}()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, c)
		}
	}()

	fb := &fakeBridge{}
	var st statusLog
	b := newEQBridge(9, 0, bin, t.TempDir(), *sampleEQForStreams(), fb, st.report)
	b.rt = directRT()
	b.addr = ln.Addr().String()
	// No tune: production timings.
	start := time.Now()
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()
	if !waitFor(t, 15*time.Second, func() bool { s, _ := fb.counts(); return s == 1 }) {
		t.Fatal("no fallback")
	}
	elapsed := time.Since(start)
	t.Logf("fallback after %v (%d CamillaDSP starts)", elapsed, starts())
	if elapsed > 7*time.Second {
		t.Errorf("fallback took %v, want < 7 s (calculated ~5.7 s)", elapsed)
	}
	if starts() != camillaMaxFails {
		t.Errorf("CamillaDSP started %d times, want %d", starts(), camillaMaxFails)
	}
	if errs := logs.errors(); len(errs) != 1 || !strings.Contains(errs[0], "falling back to alsaloop") {
		t.Errorf("ERROR lines for the event: %q, want exactly the fallback one", errs)
	}
	if !st.has(models.EQStateFailed) {
		t.Error("failed status not reported")
	}
}

// The missing-binary path also logs one ERROR.
func TestEQBridge_MissingBinarySingleError(t *testing.T) {
	captureAmixer(t)
	logs := captureLogs(t, "8")
	fb := &fakeBridge{}
	b := newEQBridge(8, 0, filepath.Join(t.TempDir(), "camilladsp"), t.TempDir(), *sampleEQForStreams(), fb, nil)
	b.rt = directRT()
	if err := b.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer b.Stop()
	if !waitFor(t, 5*time.Second, func() bool { s, _ := fb.counts(); return s == 1 }) {
		t.Fatal("no fallback")
	}
	if errs := logs.errors(); len(errs) != 1 {
		t.Errorf("ERROR lines: %q, want 1", errs)
	}
}
