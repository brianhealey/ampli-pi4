package system

import (
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	procUptime      = "/proc/uptime"
	kernelRelease   = "/proc/sys/kernel/osrelease"
	osReleasePath   = "/etc/os-release"
	deviceTreeModel = "/proc/device-tree/model"
)

// ReadHost returns identity and uptime information for the machine.
func ReadHost() HostStats {
	h := HostStats{
		Model:         hostModel(),
		Serial:        boardSerial(),
		OS:            osPrettyName(),
		Kernel:        readTrimmed(kernelRelease),
		Architecture:  runtime.GOARCH,
		UptimeSeconds: readUptime(),
	}

	if name, err := os.Hostname(); err == nil {
		h.Hostname = name
	}
	if h.UptimeSeconds > 0 {
		boot := time.Now().Add(-time.Duration(h.UptimeSeconds * float64(time.Second)))
		h.BootTime = boot.Format(time.RFC3339)
	}
	return h
}

// readUptime parses the first field of /proc/uptime (seconds since boot).
func readUptime() float64 {
	lines := readLines(procUptime)
	if len(lines) == 0 {
		return 0
	}
	fields := strings.Fields(lines[0])
	if len(fields) == 0 {
		return 0
	}
	return parseFloat(fields[0])
}

// hostModel reads the board model from the device tree, falling back to the
// /proc/cpuinfo "Model" line used by 32-bit Raspberry Pi kernels.
func hostModel() string {
	if model := readTrimmed(deviceTreeModel); model != "" {
		return model
	}
	for _, line := range readLines(procCPUInfo) {
		if key, value, ok := keyValue(line, ":"); ok && strings.EqualFold(key, "Model") {
			return value
		}
	}
	return ""
}

// boardSerial reads the Raspberry Pi board serial from /proc/cpuinfo.
func boardSerial() string {
	for _, line := range readLines(procCPUInfo) {
		if key, value, ok := keyValue(line, ":"); ok && strings.EqualFold(key, "Serial") {
			return value
		}
	}
	return ""
}

// osPrettyName reads PRETTY_NAME from /etc/os-release, e.g.
// "Debian GNU/Linux 12 (bookworm)".
func osPrettyName() string {
	for _, line := range readLines(osReleasePath) {
		key, value, ok := keyValue(line, "=")
		if !ok || key != "PRETTY_NAME" {
			continue
		}
		return strings.Trim(value, `"`)
	}
	return ""
}
