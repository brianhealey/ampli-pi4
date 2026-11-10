package streams

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// ─── vsrc → loopback PCM mapping ─────────────────────────────────────────────

// TestVirtualOutputPCM_MatchesShippedAsoundConf pins VirtualOutputPCM to the
// asound.conf and snd-aloop options that scripts/lib/30-alsa.sh installs.
func TestVirtualOutputPCM_MatchesShippedAsoundConf(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "lib", "30-alsa.sh"))
	if err != nil {
		t.Fatalf("read shipped ALSA setup: %v", err)
	}
	script := string(data)

	// snd-aloop card IDs
	m := regexp.MustCompile(`options snd-aloop [^"\n]*\bid=([\w,]+)`).FindStringSubmatch(script)
	if m == nil {
		t.Fatal("snd-aloop id= option not found in 30-alsa.sh")
	}
	if got, want := strings.Split(m[1], ","), loopbackCardIDs[:]; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("snd-aloop card IDs = %v, loopbackCardIDs = %v", got, want)
	}

	// lbNc is a plug over lbN; lbN is a dmix (playback) on hw:<card>,<device>.
	plugRe := regexp.MustCompile(`pcm\.lb(\d+)c \{ type plug; slave\.pcm "lb(\d+)"; \}`)
	dmixRe := regexp.MustCompile(`pcm\.lb(\d+) \{\s*type dmix[^}]*?slave \{ pcm "hw:(\w+),(\d+)";`)

	plugs := map[int]int{}
	for _, sm := range plugRe.FindAllStringSubmatch(script, -1) {
		c, _ := strconv.Atoi(sm[1])
		target, _ := strconv.Atoi(sm[2])
		plugs[c] = target
	}
	type pcm struct {
		card   string
		device int
	}
	dmix := map[int]pcm{}
	for _, sm := range dmixRe.FindAllStringSubmatch(script, -1) {
		n, _ := strconv.Atoi(sm[1])
		dev, _ := strconv.Atoi(sm[3])
		dmix[n] = pcm{sm[2], dev}
	}

	for vsrc := 0; vsrc < MaxVSRC; vsrc++ {
		dev := VirtualOutputDevice(vsrc)
		target, ok := plugs[vsrc]
		if !ok || target != vsrc {
			t.Errorf("%s: want plug over lb%d in asound.conf, got %v (found=%v)", dev, vsrc, target, ok)
			continue
		}
		hw, ok := dmix[vsrc]
		if !ok {
			t.Errorf("lb%d: dmix definition not found in asound.conf", vsrc)
			continue
		}
		card, device := VirtualOutputPCM(vsrc)
		if card != hw.card || device != hw.device {
			t.Errorf("VirtualOutputPCM(%d) = hw:%s,%d, asound.conf has hw:%s,%d", vsrc, card, device, hw.card, hw.device)
		}
	}
}

// ─── /proc/asound PCM state ──────────────────────────────────────────────────

func writePCMStatus(t *testing.T, root, card string, device int, subs ...string) {
	t.Helper()
	for i, status := range subs {
		dir := filepath.Join(root, card, "pcm"+strconv.Itoa(device)+"p", "sub"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlaybackPCMOpen(t *testing.T) {
	root := t.TempDir()
	writePCMStatus(t, root, "Loopback", 0, "closed\n", "closed\n")
	writePCMStatus(t, root, "Loopback", 1, "closed\n", "state: RUNNING\nowner_pid   : 1234\n")

	if open, err := playbackPCMOpen(root, "Loopback", 0); err != nil || open {
		t.Errorf("all substreams closed: open=%v err=%v, want false nil", open, err)
	}
	if open, err := playbackPCMOpen(root, "Loopback", 1); err != nil || !open {
		t.Errorf("one substream running: open=%v err=%v, want true nil", open, err)
	}
	if _, err := playbackPCMOpen(root, "Loopback3", 0); !errors.Is(err, errPCMUnknown) {
		t.Errorf("missing card: err=%v, want errPCMUnknown", err)
	}
}

func withProcAsound(t *testing.T, root string) {
	t.Helper()
	prev := procAsoundRoot
	procAsoundRoot = root
	t.Cleanup(func() { procAsoundRoot = prev })
}

func TestAudioOutputCheck(t *testing.T) {
	root := t.TempDir()
	withProcAsound(t, root)

	// vsrc 6 → lb6c → hw:Loopback,1
	check := audioOutputCheck(6)
	if err := check(); err != nil {
		t.Errorf("no PCM state available: check() = %v, want nil (no-op)", err)
	}

	writePCMStatus(t, root, "Loopback", 1, "closed\n", "closed\n")
	err := check()
	if err == nil || !strings.Contains(err.Error(), "lb6c") || !strings.Contains(err.Error(), "hw:Loopback,1") {
		t.Errorf("closed PCM: check() = %v, want an error naming lb6c and hw:Loopback,1", err)
	}

	writePCMStatus(t, root, "Loopback", 1, "state: PREPARED\n")
	if err := check(); err != nil {
		t.Errorf("open PCM: check() = %v, want nil", err)
	}
}

func TestAudioOutputCheck_NoProcAsound(t *testing.T) {
	withProcAsound(t, filepath.Join(t.TempDir(), "does-not-exist"))
	for vsrc := 0; vsrc < MaxVSRC; vsrc++ {
		if err := audioOutputCheck(vsrc)(); err != nil {
			t.Errorf("vsrc %d: check() = %v without /proc/asound, want nil", vsrc, err)
		}
	}
}

// ─── Supervisor health check ─────────────────────────────────────────────────

func newCountingSupervisor(name string, args ...string) (*Supervisor, func() int) {
	var mu sync.Mutex
	calls := 0
	sup := NewSupervisor(name, func() *exec.Cmd {
		mu.Lock()
		calls++
		mu.Unlock()
		return exec.Command(args[0], args[1:]...)
	})
	return sup, func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
}

func TestSupervisor_HealthCheckKillsAndRestarts(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	sup, calls := newCountingSupervisor("test-unhealthy", "sleep", "30")
	sup.retryForever = true
	sup.initialBackoff = 10 * time.Millisecond
	sup.maxBackoff = 20 * time.Millisecond
	sup.healthCheck = func() error { return errors.New("audio output not open") }
	sup.healthGrace = 0
	sup.healthInterval = 20 * time.Millisecond
	sup.healthFailures = 2

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sup.Stop() }()

	if !waitFor(t, 10*time.Second, func() bool { return calls() >= 3 }) {
		t.Fatalf("unhealthy process restarted %d times, want at least 2 restarts", calls()-1)
	}
	sup.mu.Lock()
	failCount := sup.failCount
	sup.mu.Unlock()
	if failCount < 1 {
		t.Errorf("failCount = %d: a kill by the health check should count as a fast failure", failCount)
	}
}

func TestSupervisor_HealthyProcessNotKilled(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	sup, calls := newCountingSupervisor("test-healthy", "sleep", "30")
	var checks sync.WaitGroup
	checks.Add(3)
	var once [3]sync.Once
	n := 0
	var nmu sync.Mutex
	sup.healthCheck = func() error {
		nmu.Lock()
		if n < 3 {
			once[n].Do(checks.Done)
			n++
		}
		nmu.Unlock()
		return nil
	}
	sup.healthGrace = 0
	sup.healthInterval = 10 * time.Millisecond

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sup.Stop() }()
	checks.Wait()
	if calls() != 1 {
		t.Errorf("healthy process started %d times, want 1", calls())
	}
	if sup.Pid() == 0 {
		t.Error("healthy process is not running")
	}
}

func TestSupervisor_HealthCheckRespectsGrace(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	sup, calls := newCountingSupervisor("test-grace", "sleep", "30")
	var mu sync.Mutex
	checked := false
	sup.healthCheck = func() error {
		mu.Lock()
		checked = true
		mu.Unlock()
		return errors.New("closed")
	}
	sup.healthGrace = time.Hour
	sup.healthInterval = 5 * time.Millisecond

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sup.Stop() }()
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if checked || calls() != 1 {
		t.Errorf("health check ran during grace period (checked=%v, starts=%d)", checked, calls())
	}
}

// ─── Fast exit with status 0 ─────────────────────────────────────────────────

// VLC exits 0 when its URL fails and vlc://quit fires. That must be retried
// forever with growing backoff, like any other fast failure.
func TestSupervisor_RetryForeverFastCleanExitBacksOff(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true not available")
	}
	var mu sync.Mutex
	var starts []time.Time
	sup := NewSupervisor("test-exit0", func() *exec.Cmd {
		mu.Lock()
		starts = append(starts, time.Now())
		mu.Unlock()
		return exec.Command(truePath)
	})
	sup.retryForever = true
	sup.maxFails = 2
	sup.initialBackoff = 10 * time.Millisecond
	sup.maxBackoff = 160 * time.Millisecond

	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sup.Stop() }()

	const want = 7
	if !waitFor(t, 10*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(starts) >= want
	}) {
		t.Fatalf("clean fast exits were not retried (%d starts)", len(starts))
	}
	if !sup.isRunning() {
		t.Fatal("supervisor gave up on status-0 exits despite retryForever")
	}

	sup.mu.Lock()
	failCount, backoff := sup.failCount, sup.backoff
	sup.mu.Unlock()
	if failCount < want-1 {
		t.Errorf("failCount = %d, want >= %d: a fast status-0 exit must count as a fast failure", failCount, want-1)
	}
	if backoff != sup.maxBackoff {
		t.Errorf("backoff = %v, want it grown to the cap %v (not reset)", backoff, sup.maxBackoff)
	}

	// Gaps between starts grow: 20ms, 40ms, 80ms, 160ms, 160ms ...
	mu.Lock()
	defer mu.Unlock()
	first := starts[1].Sub(starts[0])
	last := starts[want-1].Sub(starts[want-2])
	if last < 120*time.Millisecond || last <= first {
		t.Errorf("restart gaps did not grow: first %v, last %v", first, last)
	}
}

func TestNewSupervisor_LogsStderrByDefault(t *testing.T) {
	if !NewSupervisor("x", func() *exec.Cmd { return nil }).logStderr {
		t.Error("stderr capture should be on for every supervised child")
	}
}

// ─── Deterministic vsrc assignment ───────────────────────────────────────────

func TestManager_VSRCAssignmentFollowsStreamIDOrder(t *testing.T) {
	ids := []int{1007, 1001, 1012, 1003, 1009, 1005}
	sorted := []int{1001, 1003, 1005, 1007, 1009, 1012}

	var streams []models.Stream
	for _, id := range ids {
		streams = append(streams, models.Stream{ID: id, Name: "Radio" + strconv.Itoa(id), Type: "internet_radio",
			Config: map[string]interface{}{"url": "http://example.com/" + strconv.Itoa(id)}})
	}

	// Map iteration order is random; repeat so an order dependence shows up.
	for run := 0; run < 15; run++ {
		m := NewManager(t.TempDir(), nil)
		ctx := context.Background()
		if err := m.Sync(ctx, streams, nil); err != nil {
			t.Fatalf("Sync: %v", err)
		}
		m.mu.Lock()
		for want, id := range sorted {
			if got := m.streams[id].VSRC; got != want {
				t.Errorf("run %d: stream %d on vsrc %d, want %d", run, id, got, want)
			}
		}
		m.mu.Unlock()
		_ = m.Shutdown(ctx)
	}
}
