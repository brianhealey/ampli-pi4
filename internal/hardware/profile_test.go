package hardware_test

import (
	"context"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/hardware"
)

func TestMockProfile(t *testing.T) {
	p := hardware.MockProfile()

	if p.TotalZones != 6 {
		t.Errorf("TotalZones = %d, want 6", p.TotalZones)
	}
	if p.TotalSources != 4 {
		t.Errorf("TotalSources = %d, want 4", p.TotalSources)
	}
	if !p.HasMainUnit() {
		t.Error("HasMainUnit() = false, want true")
	}
	if !p.StreamAvailable("rca") {
		t.Error("StreamAvailable(rca) = false, want true")
	}
	if !p.StreamAvailable("aux") {
		t.Error("StreamAvailable(aux) = false, want true")
	}
	if len(p.Units) == 0 {
		t.Error("Units is empty, want at least 1")
	}
	if p.Units[0].Board.UnitType != hardware.UnitTypeMain {
		t.Errorf("Units[0].UnitType = %v, want Main", p.Units[0].Board.UnitType)
	}
	if !p.Units[0].Rev4Plus {
		t.Error("Units[0].Rev4Plus = false, want true")
	}
}

func TestDetect_Mock(t *testing.T) {
	drv := hardware.NewMock()
	if err := drv.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	p, err := hardware.Detect(context.Background(), drv)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if p == nil {
		t.Fatal("Detect returned nil profile")
	}
	if p.TotalZones != 6 {
		t.Errorf("TotalZones = %d, want 6", p.TotalZones)
	}
	if p.TotalSources != 4 {
		t.Errorf("TotalSources = %d, want 4", p.TotalSources)
	}
}

func TestParseBoardInfo_Valid(t *testing.T) {
	// Known good EEPROM bytes:
	// format=0x00, serial=0x00000123=291, unit_type=0x01=Main, board_type=0x00, rev=4,'A'
	data := [16]byte{0x00, 0x00, 0x00, 0x01, 0x23, 0x01, 0x00, 0x04, 'A'}

	info, err := hardware.ParseBoardInfo(data)
	if err != nil {
		t.Fatalf("ParseBoardInfo: %v", err)
	}
	if info.Serial != 0x123 {
		t.Errorf("Serial = 0x%x, want 0x123", info.Serial)
	}
	if info.UnitType != hardware.UnitTypeMain {
		t.Errorf("UnitType = %v, want Main", info.UnitType)
	}
	if info.BoardRev != "Rev4.A" {
		t.Errorf("BoardRev = %q, want %q", info.BoardRev, "Rev4.A")
	}
}

func TestParseBoardInfo_InvalidFormat(t *testing.T) {
	data := [16]byte{0xFF} // format byte != 0x00
	_, err := hardware.ParseBoardInfo(data)
	if err == nil {
		t.Error("ParseBoardInfo with invalid format byte should return error")
	}
}

func TestParseBoardInfo_Expansion(t *testing.T) {
	// unit_type = 0x00 = Expansion
	data := [16]byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x03, 'B'}
	info, err := hardware.ParseBoardInfo(data)
	if err != nil {
		t.Fatalf("ParseBoardInfo: %v", err)
	}
	if info.UnitType != hardware.UnitTypeExpansion {
		t.Errorf("UnitType = %v, want Expansion", info.UnitType)
	}
	if info.BoardRev != "Rev3.B" {
		t.Errorf("BoardRev = %q, want %q", info.BoardRev, "Rev3.B")
	}
}

func TestHardwareProfile_ExpansionNoAnalog(t *testing.T) {
	// Profile with only an expansion unit → TotalSources = 0, HasMainUnit = false
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{
				Index:     0,
				I2CAddr:   0x08,
				Board:     hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion},
				ZoneBase:  0,
				ZoneCount: 6,
				HasAnalog: false,
			},
		},
		TotalZones:   6,
		TotalSources: 0,
	}

	if p.HasMainUnit() {
		t.Error("HasMainUnit() = true for expansion-only profile, want false")
	}
	if p.TotalSources != 0 {
		t.Errorf("TotalSources = %d, want 0 for expansion unit", p.TotalSources)
	}
}

func TestHardwareProfile_MultiUnit(t *testing.T) {
	// Profile with main + 2 expanders → TotalZones = 18
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{Index: 0, ZoneBase: 0, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
			{Index: 1, ZoneBase: 6, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
			{Index: 2, ZoneBase: 12, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
		},
		TotalZones:   18,
		TotalSources: 4,
	}

	if p.TotalZones != 18 {
		t.Errorf("TotalZones = %d, want 18", p.TotalZones)
	}
	if !p.HasMainUnit() {
		t.Error("HasMainUnit() = false, want true")
	}
	if p.PrimaryUnitType() != hardware.UnitTypeMain {
		t.Errorf("PrimaryUnitType() = %v, want Main", p.PrimaryUnitType())
	}
}

func TestStreamAvailable_AlwaysAvailable(t *testing.T) {
	// rca and aux always available even if not in Streams list
	p := &hardware.HardwareProfile{Streams: []hardware.StreamCapability{}}
	if !p.StreamAvailable("rca") {
		t.Error("StreamAvailable(rca) = false, want true (always available)")
	}
	if !p.StreamAvailable("aux") {
		t.Error("StreamAvailable(aux) = false, want true (always available)")
	}
}

func TestStreamAvailable_Unavailable(t *testing.T) {
	p := &hardware.HardwareProfile{
		Streams: []hardware.StreamCapability{
			{Type: "pandora", Available: false, Reason: "binary not found"},
		},
	}
	if p.StreamAvailable("pandora") {
		t.Error("StreamAvailable(pandora) = true, want false")
	}
}

func TestStreamAvailable_Available(t *testing.T) {
	p := &hardware.HardwareProfile{
		Streams: []hardware.StreamCapability{
			{Type: "pandora", Available: true, Binary: "/usr/bin/pianobar"},
		},
	}
	if !p.StreamAvailable("pandora") {
		t.Error("StreamAvailable(pandora) = false, want true")
	}
}

func TestAvailableStreamTypes(t *testing.T) {
	p := &hardware.HardwareProfile{
		Streams: []hardware.StreamCapability{
			{Type: "pandora", Available: true, Binary: "/usr/bin/pianobar"},
			{Type: "airplay", Available: false, Reason: "binary not found"},
			{Type: "rca", Available: true},
		},
	}
	types := p.AvailableStreamTypes()
	if len(types) != 2 {
		t.Errorf("AvailableStreamTypes() = %v, want 2 items", types)
	}
	// Check pandora is included
	foundPandora := false
	for _, t_ := range types {
		if t_ == "pandora" {
			foundPandora = true
		}
	}
	if !foundPandora {
		t.Error("pandora not in AvailableStreamTypes() even though Available=true")
	}
}

func TestHardwareProfile_PrimaryUnitType_Empty(t *testing.T) {
	p := &hardware.HardwareProfile{}
	if p.PrimaryUnitType() != hardware.UnitTypeUnknown {
		t.Errorf("PrimaryUnitType() on empty profile = %v, want Unknown", p.PrimaryUnitType())
	}
}

func TestUnitTypeString(t *testing.T) {
	tests := []struct {
		typ  hardware.UnitType
		want string
	}{
		{hardware.UnitTypeMain, "main"},
		{hardware.UnitTypeExpansion, "expansion"},
		{hardware.UnitTypeStreamer, "streamer"},
		{hardware.UnitTypeUnknown, "unknown"},
	}
	for _, tc := range tests {
		got := tc.typ.String()
		if got != tc.want {
			t.Errorf("UnitType(%d).String() = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

func TestFanModeString(t *testing.T) {
	tests := []struct {
		mode hardware.FanMode
		want string
	}{
		{hardware.FanModeExternal, "external"},
		{hardware.FanModePWM, "pwm"},
		{hardware.FanModeLinear, "linear"},
		{hardware.FanModeForced, "forced"},
	}
	for _, tc := range tests {
		got := tc.mode.String()
		if got != tc.want {
			t.Errorf("FanMode(%d).String() = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestDisplayTypeString(t *testing.T) {
	tests := []struct {
		dt   hardware.DisplayType
		want string
	}{
		{hardware.DisplayNone, "none"},
		{hardware.DisplayTFT, "tft"},
		{hardware.DisplayEInk, "eink"},
	}
	for _, tc := range tests {
		got := tc.dt.String()
		if got != tc.want {
			t.Errorf("DisplayType(%d).String() = %q, want %q", tc.dt, got, tc.want)
		}
	}
}
