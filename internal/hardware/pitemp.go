package hardware

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const piTempPath = "/sys/class/thermal/thermal_zone0/temp"

// readPiTemp reads the Raspberry Pi CPU temperature from the thermal zone file.
// Returns temperature in Celsius. Returns an error if the file cannot be read.
func readPiTemp() (float32, error) {
	data, err := os.ReadFile(piTempPath)
	if err != nil {
		return 0, fmt.Errorf("pitemp: read %s: %w", piTempPath, err)
	}
	millideg, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("pitemp: parse: %w", err)
	}
	return float32(millideg) / 1000.0, nil
}

// RunPiTempSender is a goroutine that periodically reads the Pi CPU temperature
// and writes it to all units' REG_PI_TEMP register so the firmware's fan control
// algorithm can include it.
func RunPiTempSender(ctx context.Context, hw Driver) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tempC, err := readPiTemp()
			if err != nil {
				// Not fatal: thermal zone file may not exist on non-Pi hardware
				continue
			}
			for _, unit := range hw.Units() {
				_ = hw.WriteRPiTemp(ctx, unit, tempC)
			}
		}
	}
}
