package system

import (
	"os"

	"golang.org/x/sys/unix"
)

// mountTarget names a filesystem to report on. Targets that do not exist or
// cannot be stat'd are skipped rather than reported as empty.
type mountTarget struct {
	Path  string
	Label string
}

// defaultMounts are the filesystems worth surfacing on an AmpliPi device.
var defaultMounts = []mountTarget{
	{Path: "/", Label: "Root"},
	{Path: "/boot/firmware", Label: "Boot"},
	{Path: "/boot", Label: "Boot"},
	{Path: "/tmp", Label: "Temp"},
}

// ReadDisks returns capacity statistics for the default mounts plus configDir.
// Duplicate filesystems (same device) are reported once, first label winning.
func ReadDisks(configDir string) []DiskStats {
	targets := defaultMounts
	if configDir != "" {
		targets = append(append([]mountTarget{}, targets...), mountTarget{Path: configDir, Label: "Config"})
	}

	seen := make(map[uint64]bool, len(targets))
	out := make([]DiskStats, 0, len(targets))
	for _, t := range targets {
		stats, fsid, ok := statMount(t)
		if !ok || seen[fsid] {
			continue
		}
		seen[fsid] = true
		out = append(out, stats)
	}
	return out
}

// statMount runs statfs on one target, returning its stats and the device ID
// used to deduplicate paths that resolve to the same filesystem.
func statMount(t mountTarget) (DiskStats, uint64, bool) {
	info, err := os.Stat(t.Path)
	if err != nil || !info.IsDir() {
		return DiskStats{}, 0, false
	}

	// Dev is uint64 on Linux and int32 on Darwin; widen before comparing.
	var meta unix.Stat_t
	if err := unix.Stat(t.Path, &meta); err != nil {
		return DiskStats{}, 0, false
	}
	dev := uint64(meta.Dev)

	var st unix.Statfs_t
	if err := unix.Statfs(t.Path, &st); err != nil {
		return DiskStats{}, 0, false
	}

	// Bsize is int64 on Linux and int32 on Darwin; widen before arithmetic.
	blockSize := uint64(st.Bsize)
	total := st.Blocks * blockSize
	free := st.Bavail * blockSize
	if total == 0 {
		return DiskStats{}, 0, false
	}

	// Used is measured against space reachable by unprivileged writers, which
	// is what a "disk full" condition actually means for the daemon.
	used := (st.Blocks - st.Bfree) * blockSize
	d := DiskStats{
		Mount:       t.Path,
		Label:       t.Label,
		TotalBytes:  total,
		UsedBytes:   used,
		FreeBytes:   free,
		UsedPercent: float64(used) / float64(total) * 100.0,
	}

	return d, dev, true
}
