package system

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const thermalZone0 = "/sys/class/thermal/thermal_zone0/temp"

// Raspberry Pi `vcgencmd get_throttled` bit meanings.
const (
	bitUnderVoltageNow  = 1 << 0
	bitFreqCappedNow    = 1 << 1
	bitThrottledNow     = 1 << 2
	bitSoftTempLimitNow = 1 << 3
	bitUnderVoltageEver = 1 << 16
	bitThrottledEver    = 1 << 18
)

// vcgencmdTimeout bounds the firmware mailbox call so a wedged GPU driver
// cannot stall a stats request.
const vcgencmdTimeout = 500 * time.Millisecond

// ReadThermal returns CPU temperature and Raspberry Pi throttling state.
func ReadThermal(ctx context.Context) ThermalStats {
	t := ThermalStats{}

	// thermal_zone0 reports millidegrees Celsius.
	if milli := readTrimmed(thermalZone0); milli != "" {
		t.CPUTempC = parseFloat(milli) / 1000.0
	}

	flags, raw, ok := readThrottleFlags(ctx)
	if !ok {
		return t
	}
	t.RawThrottleHex = raw
	t.UnderVoltage = flags&bitUnderVoltageNow != 0
	t.FreqCapped = flags&bitFreqCappedNow != 0
	t.Throttled = flags&bitThrottledNow != 0
	t.SoftTempLimit = flags&bitSoftTempLimitNow != 0
	t.UnderVoltEver = flags&bitUnderVoltageEver != 0
	t.ThrottledEver = flags&bitThrottledEver != 0
	return t
}

// readThrottleFlags runs `vcgencmd get_throttled`, which prints
// "throttled=0x50000". Absence of vcgencmd (non-Pi hardware) is not an error.
func readThrottleFlags(ctx context.Context) (uint64, string, bool) {
	bin, err := exec.LookPath("vcgencmd")
	if err != nil {
		return 0, "", false
	}

	cmdCtx, cancel := context.WithTimeout(ctx, vcgencmdTimeout)
	defer cancel()

	out, err := exec.CommandContext(cmdCtx, bin, "get_throttled").Output()
	if err != nil {
		return 0, "", false
	}

	_, value, ok := keyValue(strings.TrimSpace(string(out)), "=")
	if !ok {
		return 0, "", false
	}
	flags, err := strconv.ParseUint(strings.TrimPrefix(value, "0x"), 16, 64)
	if err != nil {
		return 0, "", false
	}
	return flags, value, true
}
