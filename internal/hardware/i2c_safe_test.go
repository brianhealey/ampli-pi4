//go:build linux

package hardware_test

import (
	"context"
	"testing"
	"time"

	"github.com/micro-nova/amplipi-go/internal/hardware"
)

// waitTimeout returns a channel that receives after a short test deadline.
func waitTimeout() <-chan time.Time {
	return time.After(3 * time.Second)
}

// These tests exercise I2CDriver methods that are safe to call without real hardware.
// Init() will fail since /dev/i2c-1 doesn't exist, but that failure path is useful to test.

func TestNewI2C(t *testing.T) {
	// NewI2C() just creates a struct — no hardware access
	d := hardware.NewI2C()
	if d == nil {
		t.Fatal("NewI2C() returned nil")
	}
}

func TestI2CDriver_IsReal(t *testing.T) {
	d := hardware.NewI2C()
	if !d.IsReal() {
		t.Error("I2CDriver.IsReal() = false, want true")
	}
}

func TestI2CDriver_Units_BeforeInit(t *testing.T) {
	d := hardware.NewI2C()
	// Before Init(), units should be empty
	units := d.Units()
	if len(units) != 0 {
		t.Errorf("Units() before Init = %v, want []", units)
	}
}

func TestI2CDriver_Close_BeforeInit(t *testing.T) {
	d := hardware.NewI2C()
	// Close() before Init() should be a no-op (no fds to close)
	d.Close() // should not panic
}

func TestI2CDriver_Init_NoHardware(t *testing.T) {
	d := hardware.NewI2C()
	ctx := context.Background()

	// Init() will fail on a machine without /dev/i2c-1 or without AmpliPi hardware
	err := d.Init(ctx)
	if err == nil {
		// If we somehow have hardware, skip this test
		t.Skip("hardware detected; skipping no-hardware Init test")
	}
	// Error is expected — no preamp units should be found
	t.Logf("Init() returned expected error: %v", err)

	// After failed Init(), Units() should still return empty
	units := d.Units()
	if len(units) != 0 {
		t.Errorf("Units() after failed Init = %v, want []", units)
	}
}

func TestI2CDriver_ImplementsDriver(t *testing.T) {
	// Compile-time check: I2CDriver must implement Driver
	var _ hardware.Driver = hardware.NewI2C()
}

func TestRunPiTempSender_ContextCancel(t *testing.T) {
	m := hardware.NewMock()
	ctx, cancel := context.WithCancel(context.Background())

	// Start the sender goroutine
	done := make(chan struct{})
	go func() {
		hardware.RunPiTempSender(ctx, m)
		close(done)
	}()

	// Cancel immediately — goroutine should exit via ctx.Done() branch
	cancel()

	select {
	case <-done:
		// OK — goroutine exited cleanly
	case <-waitTimeout():
		t.Error("RunPiTempSender did not exit within timeout after context cancellation")
	}
}
