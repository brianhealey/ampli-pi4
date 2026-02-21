// Package hardware provides the hardware abstraction layer for AmpliPi.
// It defines the Driver interface and helper types used by both the real
// I2C driver and the mock driver.
package hardware

import "context"

// Register is an I2C register address.
type Register = byte

// Temps holds temperature readings from the preamp board.
type Temps struct {
	Amp1C float32 // Zone 1-3 heatsink temperature, °C
	Amp2C float32 // Zone 4-6 heatsink temperature, °C
	PSU1C float32 // HV1 PSU temperature, °C
	PSU2C float32 // HV2 PSU temperature, °C
	PiC   float32 // Raspberry Pi CPU temperature, °C
}

// Power holds power rail status flags.
type Power struct {
	PG9V       bool
	EN9V       bool
	PG12V      bool
	EN12V      bool
	PG5VD      bool
	PG5VA      bool
	HV2Present bool
}

// FanStatus holds fan control state and status.
type FanStatus struct {
	Ctrl   int  // Fan control method (0=MAX6644, 1=PWM, 2=Linear, 3=Forced)
	On     bool
	OvrTmp bool
	Fail   bool
}

// Version holds firmware version information.
type Version struct {
	Major   int
	Minor   int
	GitHash [4]byte
}

// LEDState holds the desired LED state.
type LEDState struct {
	Green bool
	Red   bool
	Zones [6]bool
}

// Driver is the hardware abstraction interface for the AmpliPi preamp board.
// All operations are context-aware and safe for concurrent use.
type Driver interface {
	// Init initializes the hardware driver. Must be called before any other method.
	Init(ctx context.Context) error

	// Write writes a single byte to a register on a specific unit.
	Write(ctx context.Context, unit int, reg Register, val byte) error

	// Read reads a single byte from a register on a specific unit.
	Read(ctx context.Context, unit int, reg Register) (byte, error)

	// SetSourceTypes configures each of the 4 sources as analog (false) or digital (true).
	SetSourceTypes(ctx context.Context, unit int, analog [4]bool) error

	// SetZoneSources sets the audio source for each of the 6 zones on a unit.
	// sources[i] is the source index (0-3) for zone i.
	SetZoneSources(ctx context.Context, unit int, sources [6]int) error

	// SetZoneMutes sets the mute state for each of the 6 zones on a unit.
	SetZoneMutes(ctx context.Context, unit int, mutes [6]bool) error

	// SetAmpEnables sets the amp enable state for each of the 6 zones on a unit.
	SetAmpEnables(ctx context.Context, unit int, enables [6]bool) error

	// SetZoneVol sets the volume for a specific zone on a unit.
	// vol is in dB, range [-80, 0]. -80 is mute.
	SetZoneVol(ctx context.Context, unit, zone int, vol int) error

	// ReadTemps reads all temperature sensors from a unit.
	ReadTemps(ctx context.Context, unit int) (Temps, error)

	// ReadPower reads the power rail status from a unit.
	ReadPower(ctx context.Context, unit int) (Power, error)

	// ReadFanStatus reads the fan status from a unit.
	ReadFanStatus(ctx context.Context, unit int) (FanStatus, error)

	// WriteRPiTemp writes the Raspberry Pi CPU temperature to the firmware
	// so it can be used in the fan control algorithm.
	WriteRPiTemp(ctx context.Context, unit int, tempC float32) error

	// ReadVersion reads the firmware version from a unit.
	ReadVersion(ctx context.Context, unit int) (Version, error)

	// SetLEDOverride enables or disables software LED control override.
	SetLEDOverride(ctx context.Context, unit int, enable bool) error

	// SetLEDState sets the LED state when override is enabled.
	SetLEDState(ctx context.Context, unit int, leds LEDState) error

	// Units returns the list of detected unit indices (0 = master, 1+ = expanders).
	Units() []int

	// IsReal returns true for a real hardware driver, false for a mock.
	IsReal() bool
}
