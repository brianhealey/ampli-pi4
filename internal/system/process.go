package system

import (
	"os"
	"runtime"
	"time"
)

// processStart is captured at package initialisation, which happens well
// before the daemon serves traffic — close enough to process start.
var processStart = time.Now()

// ReadProcess returns runtime statistics for the AmpliPi daemon itself.
func ReadProcess() ProcessStats {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return ProcessStats{
		PID:            os.Getpid(),
		GoVersion:      runtime.Version(),
		Goroutines:     runtime.NumGoroutine(),
		HeapAllocBytes: mem.HeapAlloc,
		HeapSysBytes:   mem.HeapSys,
		StackSysBytes:  mem.StackSys,
		GCCount:        mem.NumGC,
		UptimeSeconds:  time.Since(processStart).Seconds(),
	}
}
