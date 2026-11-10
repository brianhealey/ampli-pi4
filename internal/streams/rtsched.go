package streams

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Real-time scheduling for CamillaDSP bridges.
//
// alsaloop puts itself on SCHED_RR 99 (it calls sched_setscheduler under the
// unit's LimitRTPRIO=99). CamillaDSP does not, so without help every thread,
// including AlsaCapture and AlsaPlayback, runs SCHED_OTHER next to the Go
// daemon, the web UI and the stream decoders.
//
// It is launched through chrt(1) (util-linux): chrt sets the policy on itself
// and then execs the command (`chrt [options] priority command [args]`), so
// the PID the supervisor sees, and whose process group it kills, is
// CamillaDSP's own, and every thread CamillaDSP creates inherits the policy.
// Setting the policy from Go cannot cover that reliably: os/exec has no
// pre-exec hook, and setting it on the child after Start races with the
// threads CamillaDSP spawns while it opens its devices.
//
// chrt exits non-zero before exec when it may not set the policy (no
// RLIMIT_RTPRIO), which the supervisor would count as a failed CamillaDSP.
// So the policy is probed once per daemon run with `chrt --rr <prio> true`;
// if chrt is missing or the probe fails, CamillaDSP is launched directly at
// normal priority with one WARN.

// camillaRTPriority is the SCHED_RR priority for CamillaDSP. Below the
// kernel's threaded IRQ handlers (SCHED_FIFO 50), which feed the I2S and USB
// DACs and the loopback timers it depends on, and well below alsaloop's 99,
// so the alsaloop bridges of other sources always preempt it; above every
// SCHED_OTHER task, so ordinary load (decoders, the daemon, the web UI) cannot
// delay its ALSA reads and writes. It uses about 5% of a core, so at this
// level it cannot starve anything (and kernel RT throttling caps RT tasks at
// 95% regardless).
const camillaRTPriority = 40

// rtLauncher wraps a command with chrt when real-time scheduling works on this
// unit. The probe runs once; tests build their own launcher.
type rtLauncher struct {
	findChrt func() string                     // "" when chrt is not installed
	probe    func(chrt string, prio int) error // nil when the policy can be set
	prio     int

	once sync.Once
	chrt string // after once: chrt path, or "" to launch directly
}

// camillaRT is the launcher used by CamillaDSP bridges.
var camillaRT = &rtLauncher{findChrt: findChrt, probe: probeChrt, prio: camillaRTPriority}

// findChrt resolves chrt the same way as other helper binaries.
func findChrt() string {
	p := findBinary("chrt")
	if !filepath.IsAbs(p) {
		return "" // findBinary returns the bare name when nothing was found
	}
	return p
}

// probeChrt checks that chrt can set SCHED_RR prio by running `true` under it.
func probeChrt(chrt string, prio int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, chrt, "--rr", strconv.Itoa(prio), "true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resolve runs the one-time probe and returns the chrt path to use, or "".
func (r *rtLauncher) resolve() string {
	r.once.Do(func() {
		chrt := r.findChrt()
		if chrt == "" {
			slog.Warn("eq: chrt not found; CamillaDSP will run without real-time priority")
			return
		}
		if err := r.probe(chrt, r.prio); err != nil {
			slog.Warn("eq: cannot set real-time priority (needs LimitRTPRIO in amplipi.service); CamillaDSP will run at normal priority",
				"chrt", chrt, "priority", r.prio, "err", err)
			return
		}
		slog.Info("eq: CamillaDSP will run with real-time priority", "policy", "SCHED_RR", "priority", r.prio)
		r.chrt = chrt
	})
	return r.chrt
}

// command returns the exec.Cmd for bin args..., under chrt when available.
func (r *rtLauncher) command(bin string, args ...string) *exec.Cmd {
	chrt := r.resolve()
	if chrt == "" {
		return exec.Command(bin, args...)
	}
	full := append([]string{"--rr", strconv.Itoa(r.prio), bin}, args...)
	return exec.Command(chrt, full...)
}
