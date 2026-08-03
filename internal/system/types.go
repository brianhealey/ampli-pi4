// Package system collects device health telemetry (CPU, memory, disk, network,
// thermal and process statistics) for the diagnostics API.
//
// Readings are sourced from the Linux procfs/sysfs interfaces used on the
// Raspberry Pi. On platforms where those files are absent (e.g. macOS during
// development) each collector degrades gracefully to zero values rather than
// failing the whole snapshot.
package system

import "time"

// Snapshot is a point-in-time view of device health.
type Snapshot struct {
	Timestamp time.Time    `json:"timestamp"`
	Host      HostStats    `json:"host"`
	CPU       CPUStats     `json:"cpu"`
	Memory    MemoryStats  `json:"memory"`
	Disks     []DiskStats  `json:"disks"`
	Network   []NetStats   `json:"network"`
	Thermal   ThermalStats `json:"thermal"`
	Process   ProcessStats `json:"process"`
}

// HostStats describes the machine the daemon runs on.
type HostStats struct {
	Hostname      string  `json:"hostname"`
	Model         string  `json:"model"`          // e.g. "Raspberry Pi 4 Model B Rev 1.4"
	Serial        string  `json:"serial"`         // Pi board serial from /proc/cpuinfo
	OS            string  `json:"os"`             // PRETTY_NAME from /etc/os-release
	Kernel        string  `json:"kernel"`         // uname -r equivalent
	Architecture  string  `json:"architecture"`   // GOARCH
	UptimeSeconds float64 `json:"uptime_seconds"` // system uptime
	BootTime      string  `json:"boot_time"`      // RFC3339, derived from uptime
}

// CPUStats holds processor identity, load and utilisation.
type CPUStats struct {
	Model        string    `json:"model"`
	Cores        int       `json:"cores"`
	UsagePercent float64   `json:"usage_percent"` // aggregate, 0-100
	PerCore      []float64 `json:"per_core"`      // per-logical-core, 0-100
	FrequencyMHz float64   `json:"frequency_mhz"` // current clock of cpu0
	LoadAvg1     float64   `json:"load_avg_1"`
	LoadAvg5     float64   `json:"load_avg_5"`
	LoadAvg15    float64   `json:"load_avg_15"`
	Processes    int       `json:"processes"` // running/total from /proc/loadavg
	TotalTasks   int       `json:"total_tasks"`
}

// MemoryStats holds RAM and swap usage in bytes.
type MemoryStats struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"` // total - available
	FreeBytes      uint64  `json:"free_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	BuffersBytes   uint64  `json:"buffers_bytes"`
	CachedBytes    uint64  `json:"cached_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	SwapPercent    float64 `json:"swap_percent"`
}

// DiskStats holds filesystem capacity for a single mount point.
type DiskStats struct {
	Mount       string  `json:"mount"`
	Label       string  `json:"label"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// NetStats holds per-interface addressing and byte counters.
type NetStats struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac"`
	Addresses []string `json:"addresses"`
	Up        bool     `json:"up"`
	RxBytes   uint64   `json:"rx_bytes"`
	TxBytes   uint64   `json:"tx_bytes"`
}

// ThermalStats holds CPU temperature and Raspberry Pi throttling flags.
type ThermalStats struct {
	CPUTempC       float64 `json:"cpu_temp_c"`
	Throttled      bool    `json:"throttled"`        // currently throttled
	UnderVoltage   bool    `json:"under_voltage"`    // currently under-volted
	FreqCapped     bool    `json:"freq_capped"`      // currently frequency-capped
	SoftTempLimit  bool    `json:"soft_temp_limit"`  // currently at soft temp limit
	ThrottledEver  bool    `json:"throttled_ever"`   // since boot
	UnderVoltEver  bool    `json:"under_volt_ever"`  // since boot
	RawThrottleHex string  `json:"raw_throttle_hex"` // vcgencmd get_throttled value
}

// ProcessStats holds runtime statistics for the AmpliPi daemon itself.
type ProcessStats struct {
	PID            int     `json:"pid"`
	GoVersion      string  `json:"go_version"`
	Goroutines     int     `json:"goroutines"`
	HeapAllocBytes uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64  `json:"heap_sys_bytes"`
	StackSysBytes  uint64  `json:"stack_sys_bytes"`
	GCCount        uint32  `json:"gc_count"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
}
