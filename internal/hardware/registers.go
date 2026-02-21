package hardware

// Register addresses matching the STM32 firmware's CmdReg enum in ctrl_i2c.c.
const (
	RegSrcAD      Register = 0x00 // Source analog/digital select (1 bit per source, 1=digital)
	RegZone321    Register = 0x01 // Zones 1-3 source routing (2 bits per zone)
	RegZone654    Register = 0x02 // Zones 4-6 source routing (2 bits per zone)
	RegMute       Register = 0x03 // Zone mute bits (1 bit per zone, 1=muted)
	RegAmpEn      Register = 0x04 // Amp enable bits (1 bit per zone, 1=enabled)
	RegVolZone1   Register = 0x05 // Zone 1 volume (0=0dB, 80=mute)
	RegVolZone2   Register = 0x06
	RegVolZone3   Register = 0x07
	RegVolZone4   Register = 0x08
	RegVolZone5   Register = 0x09
	RegVolZone6   Register = 0x0A
	RegPower      Register = 0x0B // Power rail status (read-only)
	RegFans       Register = 0x0C // Fan control/status
	RegLEDCtrl    Register = 0x0D // LED override enable
	RegLEDVal     Register = 0x0E // LED state (zones[6:1], red, green)
	RegExpansion  Register = 0x0F // Expansion port control
	RegHV1Voltage Register = 0x10 // HV1 rail voltage UQ6.2 (0.25V resolution)
	RegAmpTemp1   Register = 0x11 // Heatsink temp zones 1-3, UQ7.1+20°C
	RegHV1Temp    Register = 0x12 // HV1 PSU temperature
	RegAmpTemp2   Register = 0x13 // Heatsink temp zones 4-6
	RegPiTemp     Register = 0x14 // RPi CPU temp (written by software)
	RegFanDuty    Register = 0x15 // Fan PWM duty UQ1.7 [0.0, 1.0]
	RegFanVolts   Register = 0x16 // Fan supply voltage UQ4.3
	RegHV2Voltage Register = 0x17 // HV2 rail voltage UQ6.2
	RegHV2Temp    Register = 0x18 // HV2 PSU temperature
	// 0x19-0x1E reserved
	RegEEPROMReq    Register = 0x1F // EEPROM control: [7:4]=page, [3:1]=addr, [0]=rd/wr_n
	RegEEPROMData   Register = 0x20 // EEPROM data window (0x20-0x2F, 16 bytes)
	RegEEPROMDataEnd Register = 0x2F
	// Internal I2C presence detection
	RegIntI2C    Register = 0xF0 // Detected devices bitmap (0xF0-0xF9)
	RegIntI2CMax Register = 0xF9
	// Version info
	RegVersionMaj  Register = 0xFA
	RegVersionMin  Register = 0xFB
	RegGitHash65   Register = 0xFC
	RegGitHash43   Register = 0xFD
	RegGitHash21   Register = 0xFE
	RegGitHash0D   Register = 0xFF
)

// VolMuteReg is the register value that means "muted" (-90dB actual).
const VolMuteReg byte = 80

// DBToVolReg converts a dB volume value [-80, 0] to a register byte [0, 80].
// 0 dB → 0 (loudest), -80 dB → 80 (mute).
func DBToVolReg(db int) byte {
	if db > 0 {
		db = 0
	}
	if db < -80 {
		db = -80
	}
	return byte(-db)
}

// VolRegToDB converts a register byte [0, 80] to a dB value [-80, 0].
func VolRegToDB(reg byte) int {
	if reg > 80 {
		reg = 80
	}
	return -int(reg)
}

// TempFromReg decodes a temperature register value (UQ7.1 + 20°C format).
// Encoding: value = (tempC - 20) * 2, so tempC = value/2 + 20.
// Special: 0x00 = disconnected (returns -999), 0xFF = shorted (returns 999).
func TempFromReg(reg byte) float32 {
	if reg == 0x00 {
		return -999
	}
	if reg == 0xFF {
		return 999
	}
	return float32(reg)/2.0 + 20.0
}

// TempToReg encodes a temperature in Celsius to the UQ7.1+20 register format.
func TempToReg(tempC float32) byte {
	v := (tempC - 20.0) * 2.0
	if v < 0 {
		return 0
	}
	if v > 254 {
		return 254
	}
	return byte(v)
}

// VoltageFromReg decodes a voltage register value (UQ6.2 format, 0.25V resolution).
func VoltageFromReg(reg byte) float32 {
	return float32(reg) / 4.0
}

// PackZone321 packs zone 1-3 source indices into the REG_ZONE321 register byte.
// Register bits: [1:0]=zone1_src, [3:2]=zone2_src, [5:4]=zone3_src, [7:6]=unused.
// Zone indices are 0-based in the function signature (zone1=src1, zone2=src2, zone3=src3).
func PackZone321(src1, src2, src3 int) byte {
	return byte((src1&0x3)<<0 | (src2&0x3)<<2 | (src3&0x3)<<4)
}

// UnpackZone321 unpacks the REG_ZONE321 register byte into zone 1-3 source indices.
func UnpackZone321(b byte) (src1, src2, src3 int) {
	src1 = int(b>>0) & 0x3
	src2 = int(b>>2) & 0x3
	src3 = int(b>>4) & 0x3
	return
}

// PackZone654 packs zone 4-6 source indices into the REG_ZONE654 register byte.
// Same bit layout as ZONE321 but for zones 4-6.
func PackZone654(src4, src5, src6 int) byte {
	return byte((src4&0x3)<<0 | (src5&0x3)<<2 | (src6&0x3)<<4)
}

// UnpackZone654 unpacks the REG_ZONE654 register byte into zone 4-6 source indices.
func UnpackZone654(b byte) (src4, src5, src6 int) {
	src4 = int(b>>0) & 0x3
	src5 = int(b>>2) & 0x3
	src6 = int(b>>4) & 0x3
	return
}

// VolZoneReg returns the volume register address for the given zone index (0-based, local to unit).
func VolZoneReg(localZone int) Register {
	if localZone < 0 || localZone > 5 {
		return RegVolZone1
	}
	return Register(RegVolZone1 + byte(localZone))
}
