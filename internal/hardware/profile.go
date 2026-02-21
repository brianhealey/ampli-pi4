package hardware

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// UnitType identifies the hardware unit type from EEPROM.
type UnitType uint8

const (
	UnitTypeExpansion UnitType = 0x00 // AP1_Z6 — 6-zone expander only, no analog sources
	UnitTypeMain      UnitType = 0x01 // AP1_S4Z6 — 4 sources + 6 zones (standard)
	UnitTypeStreamer   UnitType = 0x02 // Streamer only — no amplifier zones
	UnitTypeUnknown   UnitType = 0xFF // Unprogrammed EEPROM or not readable
)

func (u UnitType) String() string {
	switch u {
	case UnitTypeExpansion:
		return "expansion"
	case UnitTypeMain:
		return "main"
	case UnitTypeStreamer:
		return "streamer"
	default:
		return "unknown"
	}
}

// FanMode describes which fan control method the power board uses.
type FanMode uint8

const (
	FanModeExternal FanMode = iota // MAX6644 IC controls fans — Go cannot influence
	FanModePWM                     // PWM via FAN_ON GPIO (Power Board 3.A)
	FanModeLinear                  // Digital potentiometer / linear voltage (Power Board 4.A+)
	FanModeForced                  // Forced 100% — set by Go, any board
)

func (f FanMode) String() string {
	switch f {
	case FanModeExternal:
		return "external"
	case FanModePWM:
		return "pwm"
	case FanModeLinear:
		return "linear"
	case FanModeForced:
		return "forced"
	default:
		return "unknown"
	}
}

// BoardInfo holds EEPROM-derived board identity.
type BoardInfo struct {
	Serial   uint32
	UnitType UnitType
	BoardRev string // e.g. "Rev4.A"
}

// UnitInfo describes a single detected preamp unit (main or expander).
type UnitInfo struct {
	Index     int   // 0 = main unit, 1-5 = expanders
	I2CAddr   uint8 // 7-bit I2C address (0x08, 0x10, 0x18...)
	Board     BoardInfo
	ZoneBase  int  // first zone index on this unit (Index * 6)
	ZoneCount int  // always 6
	HasAnalog bool // false for expansion units (UnitTypeExpansion)
	Rev4Plus  bool // true if EEPROM detected on unit's internal I2C bus
}

// StreamCapability describes whether a stream type's required binary is available.
type StreamCapability struct {
	Type      string // "pandora", "airplay", etc.
	Available bool
	Binary    string // path found, or ""
	Reason    string // if !Available, why not
}

// DisplayType describes detected front-panel display hardware.
type DisplayType uint8

const (
	DisplayNone DisplayType = iota
	DisplayTFT              // ILI9341 via SPI
	DisplayEInk             // Waveshare 2.13" V3 via SPI
)

func (d DisplayType) String() string {
	switch d {
	case DisplayTFT:
		return "tft"
	case DisplayEInk:
		return "eink"
	default:
		return "none"
	}
}

// HardwareProfile is populated once at boot by Detect() and
// is then read-only for the lifetime of the process.
type HardwareProfile struct {
	// Units: index 0 is main, 1-5 are expanders in daisy-chain order.
	Units       []UnitInfo
	TotalZones  int  // sum of ZoneCount across all units (6-36)
	TotalSources int  // 4 if main unit present, 0 if streamer-only
	IsStreamer  bool // true if UnitTypeStreamer detected

	// Fan control mode (read from REG_FANS.ctrl on main unit after init)
	FanMode FanMode

	// HV2 present (second high-voltage rail, detected from REG_POWER.hv2)
	HV2Present bool

	// Display hardware
	Display DisplayType

	// Stream binary availability
	Streams []StreamCapability

	// Firmware version on main unit
	FirmwareVersion string // "Major.Minor-GitHash"
}

// HasMainUnit returns true if the profile contains a main (AP1_S4Z6) unit.
func (p *HardwareProfile) HasMainUnit() bool {
	for _, u := range p.Units {
		if u.Board.UnitType == UnitTypeMain {
			return true
		}
	}
	return false
}

// PrimaryUnitType returns the unit type of the first detected unit.
func (p *HardwareProfile) PrimaryUnitType() UnitType {
	if len(p.Units) == 0 {
		return UnitTypeUnknown
	}
	return p.Units[0].Board.UnitType
}

// StreamAvailable returns true if the given stream type's binary is present.
// RCA and Aux are always available (hardware passthrough).
func (p *HardwareProfile) StreamAvailable(streamType string) bool {
	// Hardware passthroughs are always available
	if streamType == "rca" || streamType == "aux" {
		return true
	}
	for _, s := range p.Streams {
		if s.Type == streamType {
			return s.Available
		}
	}
	return false
}

// AvailableStreamTypes returns a slice of stream types with binaries available on this hardware.
func (p *HardwareProfile) AvailableStreamTypes() []string {
	var types []string
	for _, s := range p.Streams {
		if s.Available {
			types = append(types, s.Type)
		}
	}
	return types
}

// Detect probes the hardware and returns a populated HardwareProfile.
// Must be called after Driver.Init() so unit detection is complete.
func Detect(ctx context.Context, drv Driver) (*HardwareProfile, error) {
	if !drv.IsReal() {
		// Mock: return a sensible default profile for development
		return MockProfile(), nil
	}

	p := &HardwareProfile{}

	units := drv.Units() // []int of detected unit indices
	if len(units) == 0 {
		return nil, fmt.Errorf("no preamp units detected on I2C bus")
	}

	for _, idx := range units {
		info, err := detectUnit(ctx, drv, idx)
		if err != nil {
			return nil, fmt.Errorf("unit %d: %w", idx, err)
		}
		p.Units = append(p.Units, info)
		p.TotalZones += info.ZoneCount
	}

	// Sources: only main unit (UnitTypeMain) has analog/digital sources
	for _, u := range p.Units {
		if u.Board.UnitType == UnitTypeMain {
			p.TotalSources = 4
		}
		if u.Board.UnitType == UnitTypeStreamer {
			p.IsStreamer = true
		}
	}

	// Fan mode: read REG_FANS from unit 0
	if fanStatus, err := drv.ReadFanStatus(ctx, 0); err == nil {
		p.FanMode = FanMode(fanStatus.Ctrl)
	}

	// HV2 presence: read REG_POWER from unit 0
	if power, err := drv.ReadPower(ctx, 0); err == nil {
		p.HV2Present = power.HV2Present
	}

	// Firmware version
	if ver, err := drv.ReadVersion(ctx, 0); err == nil {
		p.FirmwareVersion = fmt.Sprintf("%d.%d-%08x",
			ver.Major, ver.Minor,
			uint32(ver.GitHash[0])<<24|uint32(ver.GitHash[1])<<16|
				uint32(ver.GitHash[2])<<8|uint32(ver.GitHash[3]))
	}

	// Display detection
	p.Display = detectDisplay()

	// Stream capabilities
	p.Streams = detectStreamCapabilities()

	return p, nil
}

// detectUnit reads EEPROM and builds UnitInfo for a single preamp unit.
func detectUnit(ctx context.Context, drv Driver, idx int) (UnitInfo, error) {
	info := UnitInfo{
		Index:     idx,
		I2CAddr:   uint8(0x08 + uint8(idx)*0x08),
		ZoneBase:  idx * 6,
		ZoneCount: 6,
	}

	// Read EEPROM page 0 (board identity)
	pageCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	data, err := ReadEEPROMPage(pageCtx, drv, idx, 0, 0)
	if err != nil {
		// EEPROM read failed — assume unknown unit type, continue
		info.Board = BoardInfo{UnitType: UnitTypeUnknown, BoardRev: "Rev?.?"}
		info.HasAnalog = false
	} else {
		board, parseErr := ParseBoardInfo(data)
		if parseErr != nil {
			// Bad format — treat as unknown
			info.Board = BoardInfo{UnitType: UnitTypeUnknown, BoardRev: "Rev?.?"}
			info.HasAnalog = false
		} else {
			info.Board = board
			info.HasAnalog = (board.UnitType == UnitTypeMain)
		}
	}

	// Rev4Plus detection: bit 1 of REG_GIT_HASH_0_D (0xFF)
	h0d, readErr := drv.Read(ctx, idx, RegGitHash0D)
	if readErr == nil {
		info.Rev4Plus = (h0d & 0x02) != 0
	}

	return info, nil
}

// detectDisplay probes for known front-panel display hardware via GPIO sysfs.
func detectDisplay() DisplayType {
	// Check if /dev/spidev0.0 exists first — both displays require SPI
	if _, err := os.Stat("/dev/spidev0.0"); err != nil {
		return DisplayNone
	}

	// TFT: ILI9341 uses GPIO24 as DC pin
	if _, err := os.Stat("/sys/class/gpio/gpio24"); err == nil {
		return DisplayTFT
	}

	// eInk: Waveshare uses GPIO17 as DC pin
	if _, err := os.Stat("/sys/class/gpio/gpio17"); err == nil {
		return DisplayEInk
	}

	return DisplayNone
}

// streamBinaries maps each stream type to the binaries to search for (in order of preference).
var streamBinaries = []struct {
	Type string
	Bins []string // try in order
}{
	{"pandora", []string{"pianobar"}},
	{"airplay", []string{"shairport-sync-ap2", "shairport-sync"}},
	{"spotify_connect", []string{"go-librespot"}},
	{"dlna", []string{"gmrender-resurrect"}},
	{"lms", []string{"squeezelite"}},
	{"fm_radio", []string{"rtl_fm"}},
	{"bluetooth", []string{"bluealsa-aplay"}},
	{"internet_radio", []string{"vlc", "cvlc"}},
	{"file_player", []string{"vlc", "cvlc"}},
	{"rca", nil}, // always available (hardware passthrough)
	{"aux", nil}, // always available (hardware passthrough)
}

// detectStreamCapabilities checks which stream types have their required binaries installed.
func detectStreamCapabilities() []StreamCapability {
	caps := make([]StreamCapability, 0, len(streamBinaries))
	for _, sb := range streamBinaries {
		cap := StreamCapability{Type: sb.Type}
		if sb.Bins == nil {
			// Hardware passthrough — always available
			cap.Available = true
		} else {
			for _, bin := range sb.Bins {
				path, err := exec.LookPath(bin)
				if err == nil {
					cap.Available = true
					cap.Binary = path
					break
				}
			}
			if !cap.Available {
				cap.Reason = "binary not found"
			}
		}
		caps = append(caps, cap)
	}
	return caps
}

// MockProfile returns a realistic main-unit hardware profile for development and testing.
func MockProfile() *HardwareProfile {
	// Build mock stream capabilities with all types "available"
	mockStreams := make([]StreamCapability, 0, len(streamBinaries))
	for _, sb := range streamBinaries {
		mockStreams = append(mockStreams, StreamCapability{
			Type:      sb.Type,
			Available: true,
			Binary:    "/usr/bin/" + func() string {
				if len(sb.Bins) > 0 {
					return sb.Bins[0]
				}
				return sb.Type
			}(),
		})
	}

	return &HardwareProfile{
		Units: []UnitInfo{
			{
				Index:   0,
				I2CAddr: 0x08,
				Board: BoardInfo{
					Serial:   0,
					UnitType: UnitTypeMain,
					BoardRev: "Rev4.A",
				},
				ZoneBase:  0,
				ZoneCount: 6,
				HasAnalog: true,
				Rev4Plus:  true,
			},
		},
		TotalZones:      6,
		TotalSources:    4,
		IsStreamer:      false,
		FanMode:         FanModePWM,
		HV2Present:      false,
		Display:         DisplayNone,
		Streams:         mockStreams,
		FirmwareVersion: "1.7-deadbeef",
	}
}
