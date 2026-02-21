package hardware_test

import (
	"testing"

	"github.com/micro-nova/amplipi-go/internal/hardware"
)

func TestDBToVolReg(t *testing.T) {
	tests := []struct {
		db  int
		reg byte
	}{
		{0, 0},
		{-1, 1},
		{-79, 79},
		{-80, 80}, // mute
		{1, 0},    // clamp above max
		{-100, 80}, // clamp below min
	}
	for _, tc := range tests {
		got := hardware.DBToVolReg(tc.db)
		if got != tc.reg {
			t.Errorf("DBToVolReg(%d) = %d, want %d", tc.db, got, tc.reg)
		}
	}
}

func TestVolRegToDB(t *testing.T) {
	tests := []struct {
		reg byte
		db  int
	}{
		{0, 0},
		{1, -1},
		{79, -79},
		{80, -80},
	}
	for _, tc := range tests {
		got := hardware.VolRegToDB(tc.reg)
		if got != tc.db {
			t.Errorf("VolRegToDB(%d) = %d, want %d", tc.reg, got, tc.db)
		}
	}
}

func TestPackUnpackZone321(t *testing.T) {
	tests := []struct {
		s1, s2, s3 int
	}{
		{0, 0, 0},
		{1, 0, 0},
		{0, 2, 0},
		{0, 0, 3},
		{1, 2, 3},
		{3, 3, 3},
	}
	for _, tc := range tests {
		packed := hardware.PackZone321(tc.s1, tc.s2, tc.s3)
		got1, got2, got3 := hardware.UnpackZone321(packed)
		if got1 != tc.s1 || got2 != tc.s2 || got3 != tc.s3 {
			t.Errorf("Pack/Unpack(%d,%d,%d) → packed=0x%02X → (%d,%d,%d)",
				tc.s1, tc.s2, tc.s3, packed, got1, got2, got3)
		}
	}
}

func TestPackUnpackZone654(t *testing.T) {
	tests := []struct {
		s4, s5, s6 int
	}{
		{0, 1, 2},
		{3, 0, 1},
		{2, 3, 0},
	}
	for _, tc := range tests {
		packed := hardware.PackZone654(tc.s4, tc.s5, tc.s6)
		got4, got5, got6 := hardware.UnpackZone654(packed)
		if got4 != tc.s4 || got5 != tc.s5 || got6 != tc.s6 {
			t.Errorf("Pack654/Unpack654(%d,%d,%d) → packed=0x%02X → (%d,%d,%d)",
				tc.s4, tc.s5, tc.s6, packed, got4, got5, got6)
		}
	}
}

func TestTempFromReg(t *testing.T) {
	// 0x00 = disconnected
	if got := hardware.TempFromReg(0x00); got != -999 {
		t.Errorf("TempFromReg(0x00) = %f, want -999", got)
	}
	// 0xFF = shorted
	if got := hardware.TempFromReg(0xFF); got != 999 {
		t.Errorf("TempFromReg(0xFF) = %f, want 999", got)
	}
	// 0x5E = 0x5E/2 + 20 = 47 + 20 = 67°C
	if got := hardware.TempFromReg(0x5E); got != 67.0 {
		t.Errorf("TempFromReg(0x5E) = %f, want 67.0", got)
	}
	// 0x02 = 1 + 20 = 21°C
	if got := hardware.TempFromReg(0x02); got != 21.0 {
		t.Errorf("TempFromReg(0x02) = %f, want 21.0", got)
	}
}

func TestMockDriver(t *testing.T) {
	m := hardware.NewMock()
	if m.IsReal() {
		t.Error("mock driver should return IsReal()=false")
	}
	units := m.Units()
	if len(units) != 1 || units[0] != 0 {
		t.Errorf("expected units=[0], got %v", units)
	}
}
