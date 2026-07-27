//go:build !linux

package hardware

import (
	"context"
	"fmt"
)

// NewI2C returns a stub driver on non-Linux platforms.
// Use NewMock() instead for local development.
func NewI2C() Driver {
	return &i2cStub{}
}

type i2cStub struct{}

func (s *i2cStub) Init(ctx context.Context) error {
	return fmt.Errorf("I2C hardware not available on this platform; use --mock")
}

func (s *i2cStub) Write(ctx context.Context, unit int, reg Register, val byte) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) Read(ctx context.Context, unit int, reg Register) (byte, error) {
	return 0, fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetSourceTypes(ctx context.Context, unit int, analog [4]bool) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetZoneSources(ctx context.Context, unit int, sources [6]int) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetZoneMutes(ctx context.Context, unit int, mutes [6]bool) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetAmpEnables(ctx context.Context, unit int, enables [6]bool) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetZoneVol(ctx context.Context, unit, zone int, vol int) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) ReadTemps(ctx context.Context, unit int) (Temps, error) {
	return Temps{}, fmt.Errorf("I2C not available")
}

func (s *i2cStub) ReadPower(ctx context.Context, unit int) (Power, error) {
	return Power{}, fmt.Errorf("I2C not available")
}

func (s *i2cStub) ReadFanStatus(ctx context.Context, unit int) (FanStatus, error) {
	return FanStatus{}, fmt.Errorf("I2C not available")
}

func (s *i2cStub) WriteRPiTemp(ctx context.Context, unit int, tempC float32) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) ReadVersion(ctx context.Context, unit int) (Version, error) {
	return Version{}, fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetLEDOverride(ctx context.Context, unit int, enable bool) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) SetLEDState(ctx context.Context, unit int, leds LEDState) error {
	return fmt.Errorf("I2C not available")
}

func (s *i2cStub) Units() []int {
	return nil
}

func (s *i2cStub) IsReal() bool {
	return false
}
