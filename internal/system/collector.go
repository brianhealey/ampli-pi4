package system

import (
	"context"
	"sync"
	"time"
)

// Collector assembles Snapshots. It holds the CPU sampler's previous reading,
// so utilisation is measured between successive Collect calls.
//
// A Collector is safe for concurrent use.
type Collector struct {
	mu        sync.Mutex
	cpu       cpuSampler
	configDir string
}

// NewCollector creates a Collector. configDir is reported as an extra
// filesystem in disk statistics; pass "" to omit it.
func NewCollector(configDir string) *Collector {
	return &Collector{configDir: configDir}
}

// Collect gathers a full device snapshot.
func (c *Collector) Collect(ctx context.Context) Snapshot {
	c.mu.Lock()
	cpu := c.cpu.Sample()
	c.mu.Unlock()

	return Snapshot{
		Timestamp: time.Now(),
		Host:      ReadHost(),
		CPU:       cpu,
		Memory:    ReadMemory(),
		Disks:     ReadDisks(c.configDir),
		Network:   ReadNetwork(),
		Thermal:   ReadThermal(ctx),
		Process:   ReadProcess(),
	}
}

// Prime takes a throwaway CPU sample so the first Collect call returns a real
// utilisation figure instead of 0%. Call it once at startup.
func (c *Collector) Prime() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cpu.Sample()
}
