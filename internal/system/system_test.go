package system

import (
	"strings"
	"testing"
)

// These tests exercise the procfs parsers with captured fixtures so the
// Linux-only collection logic is verifiable on any development machine.

func TestParseCPUTimesAndUtilisation(t *testing.T) {
	// Two consecutive /proc/stat samples from a 2-core machine.
	first := strings.Split(strings.TrimSpace(`
cpu  1000 0 500 8000 100 0 0 0 0 0
cpu0 500 0 250 4000 50 0 0 0 0 0
cpu1 500 0 250 4000 50 0 0 0 0 0
intr 12345
ctxt 67890`), "\n")

	// Aggregate advances 1000 jiffies of which 500 are idle+iowait -> 50% busy.
	second := strings.Split(strings.TrimSpace(`
cpu  1400 0 700 8400 100 0 0 0 0 0
cpu0 900 0 450 4000 50 0 0 0 0 0
cpu1 500 0 250 4400 50 0 0 0 0 0
intr 12399`), "\n")

	prev := parseCPUTimes(first)
	cur := parseCPUTimes(second)

	if len(prev) != 3 || len(cur) != 3 {
		t.Fatalf("expected aggregate + 2 cores, got prev=%d cur=%d", len(prev), len(cur))
	}

	usage := utilisation(prev, cur)
	// Aggregate: total delta 1000, idle delta 400 -> 60% busy.
	if got := usage[0]; got < 59.9 || got > 60.1 {
		t.Errorf("aggregate usage = %v, want 60", got)
	}
	// cpu0: total delta 600, idle delta 0 -> 100% busy.
	if got := usage[1]; got < 99.9 || got > 100.1 {
		t.Errorf("cpu0 usage = %v, want 100", got)
	}
	// cpu1: total delta 400, all idle -> 0% busy.
	if got := usage[2]; got != 0 {
		t.Errorf("cpu1 usage = %v, want 0", got)
	}
}

func TestUtilisationHandlesCounterReset(t *testing.T) {
	prev := []cpuTimes{{idle: 5000, total: 10000}}
	// A counter that went backwards (hotplug or rollover) must not produce a
	// wild percentage.
	cur := []cpuTimes{{idle: 10, total: 20}}

	if got := utilisation(prev, cur)[0]; got != 0 {
		t.Errorf("usage after counter reset = %v, want 0", got)
	}
}

func TestUtilisationWithoutPreviousSample(t *testing.T) {
	cur := []cpuTimes{{idle: 100, total: 200}}
	if got := utilisation(nil, cur)[0]; got != 0 {
		t.Errorf("first sample usage = %v, want 0", got)
	}
}

func TestParseLoadAvg(t *testing.T) {
	l1, l5, l15, running, total := parseLoadAvg("0.13 0.06 0.02 6/373 12345")

	if l1 != 0.13 || l5 != 0.06 || l15 != 0.02 {
		t.Errorf("load = %v/%v/%v, want 0.13/0.06/0.02", l1, l5, l15)
	}
	if running != 6 || total != 373 {
		t.Errorf("processes = %d/%d, want 6/373", running, total)
	}
}

func TestParseSoC(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// The device tree lists most specific first; the SoC is last.
		{"pi compute module", "raspberrypi,4-compute-module-s\x00brcm,bcm2711", "BCM2711"},
		{"pi 5", "raspberrypi,5-model-b\x00brcm,bcm2712", "BCM2712"},
		{"no vendor prefix", "bcm2835", "BCM2835"},
		{"empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSoC(tc.in); got != tc.want {
				t.Errorf("parseSoC(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMemoryFromFields(t *testing.T) {
	// Values as reported by a 4 GB Raspberry Pi, already scaled to bytes.
	lines := strings.Split(strings.TrimSpace(`
MemTotal:        3932856 kB
MemFree:         2513904 kB
MemAvailable:    3367152 kB
Buffers:           53876 kB
Cached:           923664 kB
SwapTotal:       2097148 kB
SwapFree:        2097148 kB`), "\n")

	m := memoryFromFields(parseMeminfo(lines))

	if m.TotalBytes != 3932856*1024 {
		t.Errorf("total = %d, want %d", m.TotalBytes, 3932856*1024)
	}
	// Used excludes reclaimable cache: total - available.
	if want := uint64((3932856 - 3367152) * 1024); m.UsedBytes != want {
		t.Errorf("used = %d, want %d", m.UsedBytes, want)
	}
	if m.UsedPercent < 14.3 || m.UsedPercent > 14.5 {
		t.Errorf("used percent = %v, want ~14.4", m.UsedPercent)
	}
	if m.SwapUsedBytes != 0 || m.SwapPercent != 0 {
		t.Errorf("swap used = %d (%v%%), want 0", m.SwapUsedBytes, m.SwapPercent)
	}
}

func TestMemoryFromFieldsWithoutMemAvailable(t *testing.T) {
	// Kernels older than 3.14 omit MemAvailable; it is approximated.
	lines := []string{
		"MemTotal:        1000 kB",
		"MemFree:          200 kB",
		"Buffers:          100 kB",
		"Cached:           300 kB",
	}

	m := memoryFromFields(parseMeminfo(lines))

	if want := uint64(600 * 1024); m.AvailableBytes != want {
		t.Errorf("available = %d, want %d", m.AvailableBytes, want)
	}
	if want := uint64(400 * 1024); m.UsedBytes != want {
		t.Errorf("used = %d, want %d", m.UsedBytes, want)
	}
}

func TestMemoryFromFieldsEmpty(t *testing.T) {
	// A machine without procfs must yield zeroes, not a panic.
	if got := memoryFromFields(nil); got.TotalBytes != 0 || got.UsedPercent != 0 {
		t.Errorf("empty meminfo = %+v, want zero value", got)
	}
}

func TestCollectorDegradesWithoutProcfs(t *testing.T) {
	// Collect must always return a usable snapshot, even where /proc is absent.
	c := NewCollector(t.TempDir())
	c.Prime()

	snap := c.Collect(t.Context())

	if snap.Timestamp.IsZero() {
		t.Error("snapshot has no timestamp")
	}
	if snap.Process.PID == 0 {
		t.Error("process stats not populated")
	}
	if snap.CPU.Cores == 0 {
		t.Error("core count not populated")
	}
	// The temp dir is on a real filesystem, so at least one disk is reported.
	if len(snap.Disks) == 0 {
		t.Error("no disks reported for the config directory")
	}
}
