package hardware

import (
	"context"
	"fmt"
	"time"
)

// ReadEEPROMPage reads one 16-byte page from a preamp unit's EEPROM via STM32 register relay.
//
// The EEPROM is not directly accessible from the Raspberry Pi — all access is proxied
// through the STM32 microcontroller using the following protocol:
//
//  1. Write REG_EEPROM_REQUEST (0x1F) with: bits[7:4]=page, bits[3:1]=i2cAddr, bit[0]=1 (read)
//  2. Poll REG_EEPROM_REQUEST until bit[0] is still 1 (read complete), timeout 200ms
//  3. Read 16 bytes from REG_EEPROM_DATA (0x20–0x2F)
//
// Parameters:
//   - unit:    preamp unit index (0=main, 1-5=expanders)
//   - page:    EEPROM page number (0-based, each page = 16 bytes)
//   - i2cAddr: EEPROM I2C address bits A0-A2 (0 for the on-board preamp EEPROM)
func ReadEEPROMPage(ctx context.Context, drv Driver, unit, page, i2cAddr int) ([16]byte, error) {
	// Step 1: Write EEPROM control register to request a read.
	// REG_EEPROM_REQUEST (0x1F) bit layout:
	//   bits [7:4] = page number (0-15)
	//   bits [3:1] = I2C address bits A0-A2
	//   bit  [0]   = 1 for read, 0 for write
	ctrl := byte((page<<4)|(i2cAddr<<1)|1)
	if err := drv.Write(ctx, unit, RegEEPROMReq, ctrl); err != nil {
		return [16]byte{}, fmt.Errorf("EEPROM request write: %w", err)
	}

	// Step 2: Poll until the STM32 signals read completion (bit 0 set), timeout 200ms.
	// The STM32 handles the EEPROM read asynchronously in its internal I2C loop
	// (every 8ms at mod8==4), so we may need to wait up to ~12ms.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		val, err := drv.Read(ctx, unit, RegEEPROMReq)
		if err != nil {
			return [16]byte{}, fmt.Errorf("EEPROM poll: %w", err)
		}
		if val&0x01 != 0 {
			break // read complete — data available in REG_EEPROM_DATA
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Step 3: Read 16 data bytes from REG_EEPROM_DATA (0x20–0x2F).
	var data [16]byte
	for i := 0; i < 16; i++ {
		b, err := drv.Read(ctx, unit, RegEEPROMData+Register(i))
		if err != nil {
			return [16]byte{}, fmt.Errorf("EEPROM data[%d]: %w", i, err)
		}
		data[i] = b
	}
	return data, nil
}

// ParseBoardInfo parses EEPROM page 0 bytes into a BoardInfo.
//
// EEPROM byte layout (big-endian, from eeprom.py):
//
//	Offset 0x00: format    (uint8)  — must be 0x00
//	Offset 0x01: serial    (uint32) — big-endian
//	Offset 0x05: unit_type (uint8)  — UnitType enum
//	Offset 0x06: board_type (uint8) — (ignored by Go, used by factory tools)
//	Offset 0x07: board_rev (uint16) — packed as (number << 8 | ord(letter))
func ParseBoardInfo(data [16]byte) (BoardInfo, error) {
	if data[0] != 0x00 {
		return BoardInfo{}, fmt.Errorf("unsupported EEPROM format: 0x%02x (expected 0x00)", data[0])
	}
	serial := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
	unitType := UnitType(data[5])
	// data[6] = board_type (ignored)
	revNum := data[7]
	revLetter := data[8]
	rev := fmt.Sprintf("Rev%d.%c", revNum, revLetter)
	return BoardInfo{
		Serial:   serial,
		UnitType: unitType,
		BoardRev: rev,
	}, nil
}
