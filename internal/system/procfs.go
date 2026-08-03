package system

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readLines reads a file and returns its lines with trailing whitespace removed.
// A missing or unreadable file yields a nil slice — callers degrade gracefully.
func readLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), " \t\r\n"))
	}
	return lines
}

// readTrimmed reads a whole file and trims surrounding whitespace and NUL bytes.
// NUL trimming matters for device-tree nodes, which are NUL-terminated.
func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.Trim(string(data), " \t\r\n\x00")
}

// readUint reads a file containing a single unsigned integer.
func readUint(path string) uint64 {
	v, err := strconv.ParseUint(readTrimmed(path), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseFloat parses a float, returning 0 on failure.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseUint parses an unsigned integer, returning 0 on failure.
func parseUint(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// keyValue splits a "Key: value" line, as used by /proc/meminfo and
// /proc/cpuinfo. ok is false when the line has no separator.
func keyValue(line, sep string) (key, value string, ok bool) {
	idx := strings.Index(line, sep)
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+len(sep):]), true
}
