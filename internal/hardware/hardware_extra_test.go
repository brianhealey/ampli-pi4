package hardware_test

import (
	"context"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/hardware"
)

func TestSetSourceTypes(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// analog[i]=false means digital → bit i set in RegSrcAD
	// analog[i]=true means analog → bit i clear in RegSrcAD
	// {true,false,true,false} → sources 0,2 analog, sources 1,3 digital
	// RegSrcAD should have bits 1 and 3 set = 0b1010
	err := m.SetSourceTypes(ctx, 0, [4]bool{true, false, true, false})
	if err != nil {
		t.Fatalf("SetSourceTypes: %v", err)
	}

	got := m.GetReg(0, hardware.RegSrcAD)
	want := byte(0b00001010) // bits 1 and 3 set (digital at indices 1 and 3)
	if got != want {
		t.Errorf("RegSrcAD = 0b%08b (0x%02X), want 0b%08b (0x%02X)", got, got, want, want)
	}
}

func TestSetSourceTypes_AllDigital(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// All false = all digital → all bits set = 0b00001111
	err := m.SetSourceTypes(ctx, 0, [4]bool{false, false, false, false})
	if err != nil {
		t.Fatalf("SetSourceTypes: %v", err)
	}

	got := m.GetReg(0, hardware.RegSrcAD)
	want := byte(0b00001111)
	if got != want {
		t.Errorf("RegSrcAD all-digital = 0b%08b, want 0b%08b", got, want)
	}
}

func TestSetSourceTypes_AllAnalog(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// All true = all analog → no bits set = 0x00
	err := m.SetSourceTypes(ctx, 0, [4]bool{true, true, true, true})
	if err != nil {
		t.Fatalf("SetSourceTypes: %v", err)
	}

	got := m.GetReg(0, hardware.RegSrcAD)
	if got != 0 {
		t.Errorf("RegSrcAD all-analog = 0x%02X, want 0x00", got)
	}
}

func TestSetZoneSources(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	sources := [6]int{1, 2, 3, 0, 1, 2}
	err := m.SetZoneSources(ctx, 0, sources)
	if err != nil {
		t.Fatalf("SetZoneSources: %v", err)
	}

	// Verify RegZone321: zone1=src1=1, zone2=src2=2, zone3=src3=3
	gotZone321 := m.GetReg(0, hardware.RegZone321)
	expectedZone321 := hardware.PackZone321(1, 2, 3)
	if gotZone321 != expectedZone321 {
		t.Errorf("RegZone321 = 0x%02X, want 0x%02X (PackZone321(1,2,3))", gotZone321, expectedZone321)
	}

	// Verify RegZone654: zone4=src0=0, zone5=src1=1, zone6=src2=2
	gotZone654 := m.GetReg(0, hardware.RegZone654)
	expectedZone654 := hardware.PackZone654(0, 1, 2)
	if gotZone654 != expectedZone654 {
		t.Errorf("RegZone654 = 0x%02X, want 0x%02X (PackZone654(0,1,2))", gotZone654, expectedZone654)
	}

	// Unpack and verify individual values
	s1, s2, s3 := hardware.UnpackZone321(gotZone321)
	if s1 != 1 || s2 != 2 || s3 != 3 {
		t.Errorf("UnpackZone321 = (%d,%d,%d), want (1,2,3)", s1, s2, s3)
	}

	s4, s5, s6 := hardware.UnpackZone654(gotZone654)
	if s4 != 0 || s5 != 1 || s6 != 2 {
		t.Errorf("UnpackZone654 = (%d,%d,%d), want (0,1,2)", s4, s5, s6)
	}
}

func TestSetZoneMutes(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	mutes := [6]bool{true, false, true, false, true, false}
	err := m.SetZoneMutes(ctx, 0, mutes)
	if err != nil {
		t.Fatalf("SetZoneMutes: %v", err)
	}

	got := m.GetReg(0, hardware.RegMute)
	// bits 0,2,4 set = 0b010101 = 21
	want := byte(0b00010101)
	if got != want {
		t.Errorf("RegMute = 0b%08b (0x%02X), want 0b%08b (0x%02X)", got, got, want, want)
	}
}

func TestSetZoneMutes_AllMuted(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	err := m.SetZoneMutes(ctx, 0, [6]bool{true, true, true, true, true, true})
	if err != nil {
		t.Fatalf("SetZoneMutes: %v", err)
	}

	got := m.GetReg(0, hardware.RegMute)
	want := byte(0b00111111) // all 6 zones muted
	if got != want {
		t.Errorf("RegMute all-muted = 0b%08b, want 0b%08b", got, want)
	}
}

func TestSetZoneMutes_AllUnmuted(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	err := m.SetZoneMutes(ctx, 0, [6]bool{false, false, false, false, false, false})
	if err != nil {
		t.Fatalf("SetZoneMutes: %v", err)
	}

	got := m.GetReg(0, hardware.RegMute)
	if got != 0 {
		t.Errorf("RegMute all-unmuted = 0x%02X, want 0x00", got)
	}
}

func TestSetAmpEnables(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Enable zones 0, 2, 4 → bits 0, 2, 4 set = 0b00010101
	enables := [6]bool{true, false, true, false, true, false}
	err := m.SetAmpEnables(ctx, 0, enables)
	if err != nil {
		t.Fatalf("SetAmpEnables: %v", err)
	}

	got := m.GetReg(0, hardware.RegAmpEn)
	want := byte(0b00010101)
	if got != want {
		t.Errorf("RegAmpEn = 0b%08b, want 0b%08b", got, want)
	}
}

func TestSetAmpEnables_AllEnabled(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	err := m.SetAmpEnables(ctx, 0, [6]bool{true, true, true, true, true, true})
	if err != nil {
		t.Fatalf("SetAmpEnables: %v", err)
	}

	got := m.GetReg(0, hardware.RegAmpEn)
	want := byte(0b00111111)
	if got != want {
		t.Errorf("RegAmpEn all-enabled = 0b%08b, want 0b%08b", got, want)
	}
}

func TestSetZoneVolAllSix(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Set 6 different volumes for all zones
	vols := [6]int{0, -10, -20, -30, -40, -80}
	for i, v := range vols {
		if err := m.SetZoneVol(ctx, 0, i, v); err != nil {
			t.Fatalf("SetZoneVol(zone=%d, vol=%d): %v", i, v, err)
		}
	}

	// Read back and verify encoding
	for i, vol := range vols {
		reg := hardware.VolZoneReg(i)
		got := m.GetReg(0, reg)
		want := hardware.DBToVolReg(vol)
		if got != want {
			t.Errorf("zone %d vol register = 0x%02X (want 0x%02X for %ddB)", i, got, want, vol)
		}
		// Verify round-trip
		gotDB := hardware.VolRegToDB(got)
		if gotDB != vol {
			t.Errorf("zone %d vol round-trip = %d, want %d", i, gotDB, vol)
		}
	}
}

func TestSetZoneVol_Clamping(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// > 0 dB should clamp to 0
	if err := m.SetZoneVol(ctx, 0, 0, 10); err != nil {
		t.Fatalf("SetZoneVol: %v", err)
	}
	got := m.GetReg(0, hardware.VolZoneReg(0))
	if got != 0 {
		t.Errorf("vol > 0 register = %d, want 0 (clamped to 0dB)", got)
	}

	// < -80 dB should clamp to -80
	if err := m.SetZoneVol(ctx, 0, 1, -100); err != nil {
		t.Fatalf("SetZoneVol: %v", err)
	}
	got = m.GetReg(0, hardware.VolZoneReg(1))
	if got != 80 {
		t.Errorf("vol < -80 register = %d, want 80 (clamped to -80dB)", got)
	}
}

func TestReadTemps(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Set known temperature register values
	// TempFromReg: reg = (tempC - 20) * 2
	// For 47.0°C: (47-20)*2 = 54 = 0x36
	// For 25.5°C: (25.5-20)*2 = 11 = 0x0B
	// For 21.0°C: (21-20)*2 = 2 = 0x02
	if err := m.Write(ctx, 0, hardware.RegAmpTemp1, 0x36); err != nil {
		t.Fatalf("Write RegAmpTemp1: %v", err)
	}
	if err := m.Write(ctx, 0, hardware.RegAmpTemp2, 0x0B); err != nil {
		t.Fatalf("Write RegAmpTemp2: %v", err)
	}
	if err := m.Write(ctx, 0, hardware.RegHV1Temp, 0x02); err != nil {
		t.Fatalf("Write RegHV1Temp: %v", err)
	}

	temps, err := m.ReadTemps(ctx, 0)
	if err != nil {
		t.Fatalf("ReadTemps: %v", err)
	}

	// Verify Amp1C: 0x36/2.0 + 20 = 27 + 20 = 47.0
	if temps.Amp1C != 47.0 {
		t.Errorf("Amp1C = %f, want 47.0", temps.Amp1C)
	}

	// Verify Amp2C: 0x0B/2.0 + 20 = 5.5 + 20 = 25.5
	if temps.Amp2C != 25.5 {
		t.Errorf("Amp2C = %f, want 25.5", temps.Amp2C)
	}

	// Verify PSU1C: 0x02/2.0 + 20 = 1 + 20 = 21.0
	if temps.PSU1C != 21.0 {
		t.Errorf("PSU1C = %f, want 21.0", temps.PSU1C)
	}
}

func TestReadTemps_Disconnected(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// 0x00 = disconnected → -999
	if err := m.Write(ctx, 0, hardware.RegAmpTemp1, 0x00); err != nil {
		t.Fatalf("Write: %v", err)
	}

	temps, err := m.ReadTemps(ctx, 0)
	if err != nil {
		t.Fatalf("ReadTemps: %v", err)
	}
	if temps.Amp1C != -999 {
		t.Errorf("Amp1C for 0x00 = %f, want -999 (disconnected)", temps.Amp1C)
	}
}

func TestReadTemps_Shorted(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// 0xFF = shorted → 999
	if err := m.Write(ctx, 0, hardware.RegAmpTemp2, 0xFF); err != nil {
		t.Fatalf("Write: %v", err)
	}

	temps, err := m.ReadTemps(ctx, 0)
	if err != nil {
		t.Fatalf("ReadTemps: %v", err)
	}
	if temps.Amp2C != 999 {
		t.Errorf("Amp2C for 0xFF = %f, want 999 (shorted)", temps.Amp2C)
	}
}

func TestReadPower(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	power, err := m.ReadPower(ctx, 0)
	if err != nil {
		t.Fatalf("ReadPower: %v", err)
	}

	// Mock returns a reasonable default power state (all power rails good)
	if !power.PG9V {
		t.Error("Power.PG9V = false, want true (mock default)")
	}
	if !power.EN9V {
		t.Error("Power.EN9V = false, want true (mock default)")
	}
	if !power.PG12V {
		t.Error("Power.PG12V = false, want true (mock default)")
	}
	if !power.EN12V {
		t.Error("Power.EN12V = false, want true (mock default)")
	}
	if !power.PG5VD {
		t.Error("Power.PG5VD = false, want true (mock default)")
	}
	if !power.PG5VA {
		t.Error("Power.PG5VA = false, want true (mock default)")
	}
}

func TestMockUnits(t *testing.T) {
	m := hardware.NewMock()
	units := m.Units()

	if len(units) != 1 {
		t.Fatalf("Units() returned %d units, want 1", len(units))
	}
	if units[0] != 0 {
		t.Errorf("Units()[0] = %d, want 0", units[0])
	}
}

func TestMockWithUnits(t *testing.T) {
	m := hardware.NewMockWithUnits([]int{0, 1, 2})
	units := m.Units()

	if len(units) != 3 {
		t.Fatalf("Units() returned %d units, want 3", len(units))
	}
}

func TestMockFailWrite(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	m.SetFailWrite(true)

	if err := m.Write(ctx, 0, hardware.RegMute, 0x00); err == nil {
		t.Error("Write with failWrite=true returned nil error")
	}
	if err := m.SetZoneMutes(ctx, 0, [6]bool{}); err == nil {
		t.Error("SetZoneMutes with failWrite=true returned nil error")
	}
	if err := m.SetZoneVol(ctx, 0, 0, -30); err == nil {
		t.Error("SetZoneVol with failWrite=true returned nil error")
	}
}

func TestMockFailRead(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	m.SetFailRead(true)

	if _, err := m.Read(ctx, 0, hardware.RegMute); err == nil {
		t.Error("Read with failRead=true returned nil error")
	}
	if _, err := m.ReadTemps(ctx, 0); err == nil {
		t.Error("ReadTemps with failRead=true returned nil error")
	}
	if _, err := m.ReadPower(ctx, 0); err == nil {
		t.Error("ReadPower with failRead=true returned nil error")
	}
}

func TestMockInit(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Errorf("Init() = %v, want nil", err)
	}
}

func TestMockIsReal(t *testing.T) {
	m := hardware.NewMock()
	if m.IsReal() {
		t.Error("Mock.IsReal() = true, want false")
	}
}

func TestWriteRPiTemp(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	if err := m.WriteRPiTemp(ctx, 0, 45.0); err != nil {
		t.Fatalf("WriteRPiTemp: %v", err)
	}

	// Should have written TempToReg(45.0) to RegPiTemp
	got := m.GetReg(0, hardware.RegPiTemp)
	want := hardware.TempToReg(45.0)
	if got != want {
		t.Errorf("RegPiTemp = %d, want %d", got, want)
	}
}

func TestReadVersion(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	ver, err := m.ReadVersion(ctx, 0)
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}
	if ver.Major != 1 || ver.Minor != 0 {
		t.Errorf("ReadVersion = %d.%d, want 1.0", ver.Major, ver.Minor)
	}
}

func TestSetLEDOverride(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	if err := m.SetLEDOverride(ctx, 0, true); err != nil {
		t.Fatalf("SetLEDOverride(true): %v", err)
	}
	if m.GetReg(0, hardware.RegLEDCtrl) != 1 {
		t.Error("RegLEDCtrl != 1 after SetLEDOverride(true)")
	}

	if err := m.SetLEDOverride(ctx, 0, false); err != nil {
		t.Fatalf("SetLEDOverride(false): %v", err)
	}
	if m.GetReg(0, hardware.RegLEDCtrl) != 0 {
		t.Error("RegLEDCtrl != 0 after SetLEDOverride(false)")
	}
}

func TestSetLEDState(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	leds := hardware.LEDState{
		Green: true,
		Red:   false,
		Zones: [6]bool{true, false, true, false, false, true},
	}
	if err := m.SetLEDState(ctx, 0, leds); err != nil {
		t.Fatalf("SetLEDState: %v", err)
	}
	// Green=bit0, Red=bit1, Zones[0]=bit2, Zones[1]=bit3, ...
	// Green(1) + Zones[0](bit2) + Zones[2](bit4) + Zones[5](bit7) = 1 + 4 + 16 + 128 = 149 = 0b10010101
	got := m.GetReg(0, hardware.RegLEDVal)
	want := byte(0b10010101)
	if got != want {
		t.Errorf("RegLEDVal = 0b%08b, want 0b%08b", got, want)
	}
}

func TestReadFanStatus(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	fan, err := m.ReadFanStatus(ctx, 0)
	if err != nil {
		t.Fatalf("ReadFanStatus: %v", err)
	}
	// Mock returns default fan status (not running)
	if fan.On {
		t.Error("FanStatus.On = true, want false (default mock)")
	}
}

func TestVolZoneReg(t *testing.T) {
	tests := []struct {
		zone int
		want hardware.Register
	}{
		{0, hardware.RegVolZone1},
		{1, hardware.RegVolZone2},
		{5, hardware.RegVolZone6},
		{-1, hardware.RegVolZone1}, // out of range → clamp to Zone1
		{99, hardware.RegVolZone1}, // out of range → clamp to Zone1
	}
	for _, tc := range tests {
		got := hardware.VolZoneReg(tc.zone)
		if got != tc.want {
			t.Errorf("VolZoneReg(%d) = 0x%02X, want 0x%02X", tc.zone, got, tc.want)
		}
	}
}

func TestVoltageFromReg(t *testing.T) {
	tests := []struct {
		reg  byte
		want float32
	}{
		{0, 0.0},
		{4, 1.0},
		{48, 12.0}, // 48/4 = 12V
		{36, 9.0},  // 36/4 = 9V
	}
	for _, tc := range tests {
		got := hardware.VoltageFromReg(tc.reg)
		if got != tc.want {
			t.Errorf("VoltageFromReg(%d) = %f, want %f", tc.reg, got, tc.want)
		}
	}
}

func TestTempToReg(t *testing.T) {
	tests := []struct {
		tempC float32
		want  byte
	}{
		{20.0, 0},   // (20-20)*2 = 0
		{21.0, 2},   // (21-20)*2 = 2
		{47.0, 54},  // (47-20)*2 = 54
		{0.0, 0},    // below 20 → clamp to 0
		{200.0, 254}, // very high → clamp to 254
	}
	for _, tc := range tests {
		got := hardware.TempToReg(tc.tempC)
		if got != tc.want {
			t.Errorf("TempToReg(%f) = %d, want %d", tc.tempC, got, tc.want)
		}
	}
}

func TestHardwareError(t *testing.T) {
	err := hardware.ErrHardware("test error message")
	if err.Error() != "test error message" {
		t.Errorf("HardwareError.Error() = %q, want %q", err.Error(), "test error message")
	}
}

func TestVolRegToDB_Clamping(t *testing.T) {
	// reg > 80 should be clamped
	got := hardware.VolRegToDB(100)
	if got != -80 {
		t.Errorf("VolRegToDB(100) = %d, want -80 (clamped)", got)
	}
}

func TestMockGetReg_MissingUnit(t *testing.T) {
	m := hardware.NewMock()
	// GetReg for a unit that doesn't exist should return 0
	got := m.GetReg(99, hardware.RegMute)
	if got != 0 {
		t.Errorf("GetReg for missing unit = %d, want 0", got)
	}
}

func TestSetZoneVol_InvalidZone(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Zone index out of range (0-5)
	if err := m.SetZoneVol(ctx, 0, 99, -30); err == nil {
		t.Error("SetZoneVol with zone=99 should return error")
	}
	if err := m.SetZoneVol(ctx, 0, -1, -30); err == nil {
		t.Error("SetZoneVol with zone=-1 should return error")
	}
}

func TestMockRead_ValidReg(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Write a value and read it back
	if err := m.Write(ctx, 0, hardware.RegMute, 0x15); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := m.Read(ctx, 0, hardware.RegMute)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != 0x15 {
		t.Errorf("Read = 0x%02X, want 0x15", got)
	}
}

func TestMockRead_MissingReg(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Reading a register that was never written returns 0
	got, err := m.Read(ctx, 0, 0x50) // some arbitrary reg
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	_ = got // just ensure no error
}

func TestMock_UnitAutoInit(t *testing.T) {
	// NewMock() only pre-initializes unit 0.
	// Writing to unit 1 should auto-initialize it (covering ensureUnit and Write's !ok branch).
	m := hardware.NewMock()
	ctx := context.Background()

	// Write to unit 1 → triggers the "!ok" branch in Write that creates the regs map
	if err := m.Write(ctx, 1, hardware.RegMute, 0x0F); err != nil {
		t.Fatalf("Write to unit 1: %v", err)
	}
	got := m.GetReg(1, hardware.RegMute)
	if got != 0x0F {
		t.Errorf("GetReg(1, RegMute) = 0x%02X, want 0x0F", got)
	}

	// SetZoneMutes to unit 1 → triggers ensureUnit for non-pre-initialized unit
	if err := m.SetZoneMutes(ctx, 1, [6]bool{true, true, false, false, false, false}); err != nil {
		t.Fatalf("SetZoneMutes unit 1: %v", err)
	}

	// SetSourceTypes to unit 1 → triggers ensureUnit
	if err := m.SetSourceTypes(ctx, 1, [4]bool{false, false, false, false}); err != nil {
		t.Fatalf("SetSourceTypes unit 1: %v", err)
	}

	// SetAmpEnables to unit 1 → triggers ensureUnit
	if err := m.SetAmpEnables(ctx, 1, [6]bool{true, true, true, false, false, false}); err != nil {
		t.Fatalf("SetAmpEnables unit 1: %v", err)
	}

	// ReadTemps from unit 1 → triggers getOrInit for non-pre-initialized unit
	_, err := m.ReadTemps(ctx, 1)
	if err != nil {
		t.Fatalf("ReadTemps unit 1: %v", err)
	}
}

func TestMock_Read_UnitNotPresent(t *testing.T) {
	m := hardware.NewMock()
	ctx := context.Background()

	// Reading from a unit with no initialized regs → returns 0, no error
	got, err := m.Read(ctx, 99, hardware.RegMute)
	if err != nil {
		t.Fatalf("Read from absent unit: %v", err)
	}
	if got != 0 {
		t.Errorf("Read from absent unit = %d, want 0", got)
	}
}

func TestMockMultiUnit(t *testing.T) {
	m := hardware.NewMockWithUnits([]int{0, 1})
	ctx := context.Background()

	// Write to unit 0 and unit 1 independently
	if err := m.SetZoneMutes(ctx, 0, [6]bool{true, false, false, false, false, false}); err != nil {
		t.Fatalf("SetZoneMutes unit 0: %v", err)
	}
	if err := m.SetZoneMutes(ctx, 1, [6]bool{false, true, false, false, false, false}); err != nil {
		t.Fatalf("SetZoneMutes unit 1: %v", err)
	}

	unit0Mute := m.GetReg(0, hardware.RegMute)
	unit1Mute := m.GetReg(1, hardware.RegMute)

	if unit0Mute != 0b00000001 {
		t.Errorf("unit 0 RegMute = 0b%08b, want 0b00000001", unit0Mute)
	}
	if unit1Mute != 0b00000010 {
		t.Errorf("unit 1 RegMute = 0b%08b, want 0b00000010", unit1Mute)
	}
}
