//go:build linux

package hardware

import (
	"fmt"
	"log/slog"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/host/v3"
)

const (
	// GPIO pins for STM32 preamp control (BCM numbering).
	//
	// Platform Compatibility (CM3+ → CM4S Migration):
	//   - These pins are IDENTICAL on both CM3+ and CM4S platforms
	//   - The AmpliPi carrier board routes these pins consistently across platforms
	//   - GPIO4 and GPIO5 are standard BCM GPIO pins, unchanged between BCM2837 (CM3+)
	//     and BCM2711 (CM4S)
	//   - The GPIO pin changes mentioned in platform migration notes refer specifically
	//     to SPI2 pins (GPIO40-45), NOT to these reset/boot control pins
	//
	// References:
	//   - amplipi-go/scripts/lib/20-hardware.sh:12-13 (confirms GPIO4/5 usage)
	//   - amplipi-go/scripts/lib/20-hardware.sh:67 (confirms carrier board compatibility)
	//   - CURRENTSTATE.md:56-57 (/boot/config.txt settings)
	//
	pinNRST  = "GPIO4" // Active-low reset signal
	pinBOOT0 = "GPIO5" // Bootloader mode selection (0=firmware, 1=bootloader)
)

// resetSTM32 performs the hardware reset sequence for the STM32 preamp board.
// This must be called before any I2C communication attempts.
//
// Reset sequence (matches legacy Python implementation in amplipi/hw.py:270-287):
//  1. Initialize GPIO host driver
//  2. Set NRST (GPIO4) low to assert reset
//  3. Set BOOT0 (GPIO5) to determine boot mode
//  4. Hold reset for 1ms (hardware requires >300ns)
//  5. Release NRST (set high) to exit reset
//  6. Wait 10ms for STM32 to complete startup (~6ms actual, 10ms for margin)
//
// bootloader: if true, STM32 enters bootloader mode for firmware updates.
//
//	if false, STM32 boots normally from flash.
func resetSTM32(bootloader bool) error {
	// Initialize periph.io GPIO host driver
	if _, err := host.Init(); err != nil {
		return fmt.Errorf("gpio: host init failed: %w", err)
	}

	// Get GPIO4 (NRST) pin handle
	nrstPin := gpioreg.ByName(pinNRST)
	if nrstPin == nil {
		return fmt.Errorf("gpio: failed to open %s (NRST)", pinNRST)
	}

	// Get GPIO5 (BOOT0) pin handle
	boot0Pin := gpioreg.ByName(pinBOOT0)
	if boot0Pin == nil {
		return fmt.Errorf("gpio: failed to open %s (BOOT0)", pinBOOT0)
	}

	// Configure NRST as output and assert reset (low)
	if err := nrstPin.Out(gpio.Low); err != nil {
		return fmt.Errorf("gpio: failed to assert NRST: %w", err)
	}

	// Configure BOOT0 as output
	// After reset, the STM32 samples BOOT0 to determine boot mode:
	//   Low  = boot from flash (normal operation)
	//   High = boot from bootloader ROM (for firmware updates)
	boot0Level := gpio.Low
	if bootloader {
		boot0Level = gpio.High
	}
	if err := boot0Pin.Out(boot0Level); err != nil {
		return fmt.Errorf("gpio: failed to set BOOT0: %w", err)
	}

	// Hold reset line low for >300ns (hardware requirement)
	// Use 1ms for generous margin
	time.Sleep(1 * time.Millisecond)

	// Release reset by driving NRST high
	if err := nrstPin.Out(gpio.High); err != nil {
		return fmt.Errorf("gpio: failed to release NRST: %w", err)
	}

	// Wait for STM32 to complete startup sequence
	// Each preamp's microcontroller takes ~6ms to initialize after releasing NRST
	// (firmware initialization: pins_init, systick, i2c, uart, timers, etc.)
	// Wait 10ms to provide margin before attempting I2C communication
	time.Sleep(10 * time.Millisecond)

	slog.Debug("gpio: STM32 reset complete",
		"nrst_pin", pinNRST,
		"boot0_pin", pinBOOT0,
		"bootloader", bootloader)

	return nil
}
