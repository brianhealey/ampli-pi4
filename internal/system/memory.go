package system

import "strings"

const procMeminfo = "/proc/meminfo"

// ReadMemory returns RAM and swap usage parsed from /proc/meminfo.
func ReadMemory() MemoryStats {
	return memoryFromFields(parseMeminfo(readLines(procMeminfo)))
}

// memoryFromFields derives usage figures from parsed /proc/meminfo values.
func memoryFromFields(fields map[string]uint64) MemoryStats {
	if len(fields) == 0 {
		return MemoryStats{}
	}

	m := MemoryStats{
		TotalBytes:     fields["MemTotal"],
		FreeBytes:      fields["MemFree"],
		AvailableBytes: fields["MemAvailable"],
		BuffersBytes:   fields["Buffers"],
		CachedBytes:    fields["Cached"],
		SwapTotalBytes: fields["SwapTotal"],
	}

	// MemAvailable is absent on kernels older than 3.14; approximate it.
	if m.AvailableBytes == 0 {
		m.AvailableBytes = m.FreeBytes + m.BuffersBytes + m.CachedBytes
	}

	if m.TotalBytes > 0 {
		// "Used" excludes reclaimable buffers/cache, matching `free -h`.
		if m.AvailableBytes <= m.TotalBytes {
			m.UsedBytes = m.TotalBytes - m.AvailableBytes
		}
		m.UsedPercent = float64(m.UsedBytes) / float64(m.TotalBytes) * 100.0
	}

	if m.SwapTotalBytes > 0 {
		if free := fields["SwapFree"]; free <= m.SwapTotalBytes {
			m.SwapUsedBytes = m.SwapTotalBytes - free
		}
		m.SwapPercent = float64(m.SwapUsedBytes) / float64(m.SwapTotalBytes) * 100.0
	}

	return m
}

// parseMeminfo parses /proc/meminfo lines into a map of byte counts.
// Values are reported in kB, so they are scaled to bytes here.
func parseMeminfo(lines []string) map[string]uint64 {
	if len(lines) == 0 {
		return nil
	}
	out := make(map[string]uint64, len(lines))
	for _, line := range lines {
		key, value, ok := keyValue(line, ":")
		if !ok {
			continue
		}
		num, unit, hasUnit := strings.Cut(value, " ")
		v := parseUint(num)
		if hasUnit && strings.EqualFold(strings.TrimSpace(unit), "kB") {
			v *= 1024
		}
		out[key] = v
	}
	return out
}
