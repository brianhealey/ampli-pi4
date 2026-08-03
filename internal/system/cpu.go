package system

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	procStat             = "/proc/stat"
	procLoadavg          = "/proc/loadavg"
	procCPUInfo          = "/proc/cpuinfo"
	cpu0FreqPath         = "/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq"
	deviceTreeCompatible = "/proc/device-tree/compatible"
)

// cpuTimes holds the jiffy counters of a single /proc/stat "cpu" line.
type cpuTimes struct {
	idle  uint64 // idle + iowait
	total uint64 // sum of all fields
}

// cpuSampler computes CPU utilisation as a delta between consecutive samples.
// It is not safe for concurrent use; the Collector serialises access.
type cpuSampler struct {
	prev []cpuTimes // index 0 is the aggregate, 1..n are per-core
}

// Sample returns current CPU statistics. Utilisation is measured against the
// previous call, so the first sample after start reports 0%.
func (s *cpuSampler) Sample() CPUStats {
	stats := CPUStats{
		Model: cpuModel(),
		Cores: runtime.NumCPU(),
	}

	cur := readCPUTimes()
	if len(cur) > 0 {
		usage := utilisation(s.prev, cur)
		if len(usage) > 0 {
			stats.UsagePercent = usage[0]
			stats.PerCore = usage[1:]
		}
		s.prev = cur
	}

	stats.LoadAvg1, stats.LoadAvg5, stats.LoadAvg15, stats.Processes, stats.TotalTasks = readLoadAvg()
	if khz := readUint(cpu0FreqPath); khz > 0 {
		stats.FrequencyMHz = float64(khz) / 1000.0
	}
	return stats
}

// readCPUTimes parses the aggregate and per-core lines from /proc/stat.
func readCPUTimes() []cpuTimes {
	return parseCPUTimes(readLines(procStat))
}

// parseCPUTimes converts /proc/stat lines into jiffy counters.
// The returned slice is ordered: index 0 aggregate, 1..n cpu0..cpuN.
func parseCPUTimes(lines []string) []cpuTimes {
	var times []cpuTimes
	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		var t cpuTimes
		for i, f := range fields[1:] {
			v := parseUint(f)
			t.total += v
			// Fields 3 and 4 (idle, iowait) both count as idle time.
			if i == 3 || i == 4 {
				t.idle += v
			}
		}
		times = append(times, t)
	}
	return times
}

// utilisation converts two sets of jiffy counters into busy percentages.
// A missing or non-advancing previous sample yields 0 for that entry.
func utilisation(prev, cur []cpuTimes) []float64 {
	out := make([]float64, len(cur))
	for i := range cur {
		if i >= len(prev) {
			continue
		}
		totalDelta := cur[i].total - prev[i].total
		idleDelta := cur[i].idle - prev[i].idle
		// Guard against counter resets (CPU hotplug) and identical samples.
		if cur[i].total < prev[i].total || totalDelta == 0 || idleDelta > totalDelta {
			continue
		}
		out[i] = float64(totalDelta-idleDelta) / float64(totalDelta) * 100.0
	}
	return out
}

// readLoadAvg reads and parses /proc/loadavg.
func readLoadAvg() (l1, l5, l15 float64, running, total int) {
	lines := readLines(procLoadavg)
	if len(lines) == 0 {
		return
	}
	return parseLoadAvg(lines[0])
}

// parseLoadAvg parses a /proc/loadavg line: "0.12 0.34 0.56 1/234 5678".
func parseLoadAvg(line string) (l1, l5, l15 float64, running, total int) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return
	}
	l1, l5, l15 = parseFloat(fields[0]), parseFloat(fields[1]), parseFloat(fields[2])
	if len(fields) >= 4 {
		if run, tot, ok := keyValue(fields[3], "/"); ok {
			running, total = int(parseUint(run)), int(parseUint(tot))
		}
	}
	return
}

// armCoreNames maps the ARM "CPU part" id to a core name. Only the parts used
// by Raspberry Pi models are listed; anything else falls back to the SoC name.
var armCoreNames = map[string]string{
	"0xb76": "ARM1176",    // Pi 1 / Zero
	"0xc07": "Cortex-A7",  // Pi 2
	"0xd03": "Cortex-A53", // Pi 3
	"0xd08": "Cortex-A72", // Pi 4 / CM4
	"0xd0b": "Cortex-A76", // Pi 5
}

// cpuModel returns a human-readable processor name.
//
// x86 and armhf kernels expose "model name" directly. Recent arm64 Raspberry Pi
// kernels expose neither that nor "Hardware", so the name is assembled from the
// SoC in the device tree and the ARM core id — e.g. "BCM2711 (Cortex-A72)".
func cpuModel() string {
	var hardware, part string
	for _, line := range readLines(procCPUInfo) {
		key, value, ok := keyValue(line, ":")
		if !ok || value == "" {
			continue
		}
		switch strings.ToLower(key) {
		case "model name":
			return value
		case "hardware":
			hardware = value
		case "cpu part":
			part = strings.ToLower(value)
		}
	}

	soc := hardware
	if soc == "" {
		soc = socFromDeviceTree()
	}
	core := armCoreNames[part]

	switch {
	case soc != "" && core != "":
		return fmt.Sprintf("%s (%s)", soc, core)
	case soc != "":
		return soc
	case core != "":
		return core
	default:
		return runtime.GOARCH
	}
}

// socFromDeviceTree reads the SoC name from the device tree's NUL-separated
// "compatible" list, whose last entry is the most generic — e.g.
// "raspberrypi,4-compute-module-s\0brcm,bcm2711" yields "BCM2711".
func socFromDeviceTree() string {
	return parseSoC(readTrimmed(deviceTreeCompatible))
}

// parseSoC extracts the SoC name from a device-tree "compatible" value.
func parseSoC(data string) string {
	if data == "" {
		return ""
	}
	entries := strings.Split(data, "\x00")
	last := entries[len(entries)-1]

	// Entries are "vendor,part"; the part alone is the recognisable name.
	if _, part, ok := keyValue(last, ","); ok && part != "" {
		return strings.ToUpper(part)
	}
	return strings.ToUpper(last)
}
