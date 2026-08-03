package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// unitProbeTimeout bounds the per-unit telemetry reads so a wedged I2C bus
// cannot stall a diagnostics request.
const unitProbeTimeout = 500 * time.Millisecond

// HardwareStatus returns the detected controller topology together with live
// telemetry (firmware version, temperatures, power rails, fans) from each unit.
//
// Units that fail to respond are still listed, marked offline with the error —
// a missing expander is exactly what a diagnostics page needs to show.
func (c *Controller) HardwareStatus(ctx context.Context) models.HardwareStatus {
	status := models.HardwareStatus{
		Units:         []models.UnitStatus{},
		StreamSupport: []models.StreamCapability{},
	}
	if c.hw != nil {
		status.Mock = !c.hw.IsReal()
	}
	if c.profile == nil {
		return status
	}

	p := c.profile
	status.Detected = true
	status.UnitCount = len(p.Units)
	status.TotalZones = p.TotalZones
	status.TotalSources = p.TotalSources
	status.IsStreamer = p.IsStreamer
	status.FanMode = p.FanMode.String()
	status.HV2Present = p.HV2Present
	status.Display = p.Display.String()
	status.FirmwareVersion = p.FirmwareVersion
	status.PhysicalOutputs = p.AvailablePhysicalOutputs
	status.StreamSupport = streamCapabilities(p.Streams)

	for _, unit := range p.Units {
		status.Units = append(status.Units, c.unitStatus(ctx, unit))
	}
	return status
}

// unitStatus builds the status of one unit, probing the hardware for live values.
func (c *Controller) unitStatus(ctx context.Context, unit hardware.UnitInfo) models.UnitStatus {
	s := models.UnitStatus{
		Index:         unit.Index,
		I2CAddress:    fmt.Sprintf("0x%02X", unit.I2CAddr),
		Type:          unit.Board.UnitType.String(),
		BoardRevision: unit.Board.BoardRev,
		Serial:        unit.Board.Serial,
		ZoneBase:      unit.ZoneBase,
		ZoneCount:     unit.ZoneCount,
		HasAnalog:     unit.HasAnalog,
		Rev4Plus:      unit.Rev4Plus,
	}
	if c.hw == nil {
		s.Error = "no hardware driver"
		return s
	}

	probeCtx, cancel := context.WithTimeout(ctx, unitProbeTimeout)
	defer cancel()

	// The version read doubles as the liveness check: if the unit answers this,
	// it is on the bus.
	ver, err := c.hw.ReadVersion(probeCtx, unit.Index)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.Online = true
	s.FirmwareVersion = formatFirmwareVersion(ver)

	if temps, err := c.hw.ReadTemps(probeCtx, unit.Index); err == nil {
		s.Temps = &models.UnitTemps{
			Amp1C: temps.Amp1C,
			Amp2C: temps.Amp2C,
			PSU1C: temps.PSU1C,
			PSU2C: temps.PSU2C,
			PiC:   temps.PiC,
		}
	}
	if power, err := c.hw.ReadPower(probeCtx, unit.Index); err == nil {
		s.Power = &models.UnitPower{
			PG9V:       power.PG9V,
			EN9V:       power.EN9V,
			PG12V:      power.PG12V,
			EN12V:      power.EN12V,
			PG5VD:      power.PG5VD,
			PG5VA:      power.PG5VA,
			HV2Present: power.HV2Present,
		}
	}
	if fans, err := c.hw.ReadFanStatus(probeCtx, unit.Index); err == nil {
		s.Fans = &models.UnitFans{
			Control:  hardware.FanMode(fans.Ctrl).String(),
			On:       fans.On,
			OverTemp: fans.OvrTmp,
			Failed:   fans.Fail,
		}
	}
	return s
}

// formatFirmwareVersion renders a firmware version as "Major.Minor-githash",
// matching the format used in the hardware profile.
func formatFirmwareVersion(v hardware.Version) string {
	return fmt.Sprintf("%d.%d-%02x%02x%02x%02x",
		v.Major, v.Minor, v.GitHash[0], v.GitHash[1], v.GitHash[2], v.GitHash[3])
}

// streamCapabilities converts hardware capability records into API models.
func streamCapabilities(caps []hardware.StreamCapability) []models.StreamCapability {
	out := make([]models.StreamCapability, 0, len(caps))
	for _, c := range caps {
		out = append(out, models.StreamCapability{
			Type:      c.Type,
			Available: c.Available,
			Binary:    c.Binary,
			Reason:    c.Reason,
		})
	}
	return out
}
