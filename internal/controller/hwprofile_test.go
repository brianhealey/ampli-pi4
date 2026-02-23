package controller_test

import (
	"context"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/controller"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// strPtr is a helper to create a *string for test literals.
func strPtr(s string) *string { return &s }

// newProfiledController creates a controller with the given hardware profile.
func newProfiledController(t *testing.T, p *hardware.HardwareProfile) *controller.Controller {
	t.Helper()
	hw := hardware.NewMock()
	store := config.NewMemStore()
	bus := events.NewBus()
	ctrl, err := controller.New(hw, p, store, bus, nil, nil)
	if err != nil {
		t.Fatalf("controller.New: %v", err)
	}
	return ctrl
}

func TestCreateStream_UnavailableType(t *testing.T) {
	// Profile with pandora not available → CreateStream("pandora") → 400 error
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
		},
		TotalSources: 4,
		TotalZones:   6,
		Streams: []hardware.StreamCapability{
			{Type: "pandora", Available: false, Reason: "binary not found"},
		},
	}
	ctrl := newProfiledController(t, p)
	ctx := context.Background()

	_, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "Radio", Type: "pandora"})
	if appErr == nil {
		t.Fatal("CreateStream with unavailable type should return error")
	}
	if appErr.Status != 400 {
		t.Errorf("expected status 400, got %d", appErr.Status)
	}
}

func TestCreateStream_AvailableType(t *testing.T) {
	// Profile with pandora available → CreateStream("pandora") should succeed
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
		},
		TotalSources: 4,
		TotalZones:   6,
		Streams: []hardware.StreamCapability{
			{Type: "pandora", Available: true, Binary: "/usr/bin/pianobar"},
		},
	}
	ctrl := newProfiledController(t, p)
	ctx := context.Background()

	_, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "My Pandora", Type: "pandora"})
	if appErr != nil {
		t.Fatalf("CreateStream with available type should succeed: %v", appErr)
	}
}

func TestCreateStream_RCAAlwaysAvailable(t *testing.T) {
	// Profile with no stream capabilities → rca should still be available
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
		},
		TotalSources: 4,
		TotalZones:   6,
		Streams:      []hardware.StreamCapability{}, // empty — no binaries found
	}
	ctrl := newProfiledController(t, p)
	ctx := context.Background()

	_, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "RCA Input", Type: "rca"})
	if appErr != nil {
		t.Fatalf("CreateStream with rca type should always succeed: %v", appErr)
	}
}

func TestSetSource_NoSourcesOnExpander(t *testing.T) {
	// Profile with expansion unit only → SetSource → 400 error
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
		},
		TotalSources: 0,
		TotalZones:   6,
	}
	ctrl := newProfiledController(t, p)
	ctx := context.Background()

	_, appErr := ctrl.SetSource(ctx, 0, models.SourceUpdate{Input: strPtr("local")})
	if appErr == nil {
		t.Fatal("SetSource on expansion-only unit should return error")
	}
}

func TestSetSource_NilProfileAllowsAll(t *testing.T) {
	// nil profile → no restrictions (test compatibility)
	ctrl := newProfiledController(t, nil)
	ctx := context.Background()

	name := "Test Source"
	_, appErr := ctrl.SetSource(ctx, 0, models.SourceUpdate{Name: &name})
	if appErr != nil {
		t.Fatalf("SetSource with nil profile should succeed: %v", appErr)
	}
}

func TestDefaultStateFromProfile_MultiUnit(t *testing.T) {
	// Main + 2 expanders → 18 zones, 4 sources
	p := &hardware.HardwareProfile{
		TotalSources: 4,
		TotalZones:   18,
		Units: []hardware.UnitInfo{
			{Index: 0, ZoneBase: 0, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
			{Index: 1, ZoneBase: 6, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
			{Index: 2, ZoneBase: 12, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
		},
	}

	state := models.DefaultStateFromProfile(p)

	if len(state.Zones) != 18 {
		t.Errorf("Zones = %d, want 18", len(state.Zones))
	}
	if len(state.Sources) != 4 {
		t.Errorf("Sources = %d, want 4", len(state.Sources))
	}

	// Verify zone IDs are sequential and correct
	if state.Zones[6].ID != 6 {
		t.Errorf("Zones[6].ID = %d, want 6 (first expander zone)", state.Zones[6].ID)
	}
	if state.Zones[12].ID != 12 {
		t.Errorf("Zones[12].ID = %d, want 12 (second expander zone)", state.Zones[12].ID)
	}
}

func TestDefaultStateFromProfile_SingleMain(t *testing.T) {
	// Single main unit → 6 zones, 4 sources
	p := hardware.MockProfile()
	state := models.DefaultStateFromProfile(p)

	if len(state.Zones) != 6 {
		t.Errorf("Zones = %d, want 6", len(state.Zones))
	}
	if len(state.Sources) != 4 {
		t.Errorf("Sources = %d, want 4", len(state.Sources))
	}
}

func TestDefaultStateFromProfile_ExpansionOnly(t *testing.T) {
	// Expansion-only unit → 6 zones, 0 sources
	p := &hardware.HardwareProfile{
		Units: []hardware.UnitInfo{
			{Index: 0, ZoneBase: 0, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
		},
		TotalZones:   6,
		TotalSources: 0,
	}

	state := models.DefaultStateFromProfile(p)

	if len(state.Zones) != 6 {
		t.Errorf("Zones = %d, want 6", len(state.Zones))
	}
	if len(state.Sources) != 0 {
		t.Errorf("Sources = %d, want 0 for expansion-only", len(state.Sources))
	}
	if len(state.Streams) != 0 {
		t.Errorf("Streams = %d, want 0 for expansion-only (no sources, no RCA)", len(state.Streams))
	}
}

func TestDefaultStateFromProfile_Nil(t *testing.T) {
	// nil profile → falls back to DefaultState()
	state := models.DefaultStateFromProfile(nil)

	if len(state.Zones) != 6 {
		t.Errorf("Zones = %d, want 6", len(state.Zones))
	}
	if len(state.Sources) != 4 {
		t.Errorf("Sources = %d, want 4", len(state.Sources))
	}
}

func TestGetInfo_WithProfile(t *testing.T) {
	p := hardware.MockProfile()
	ctrl := newProfiledController(t, p)

	info := ctrl.GetInfo()

	if info.Units != 1 {
		t.Errorf("Info.Units = %d, want 1", info.Units)
	}
	if info.Zones != 6 {
		t.Errorf("Info.Zones = %d, want 6", info.Zones)
	}
	if info.FanMode != "pwm" {
		t.Errorf("Info.FanMode = %q, want %q", info.FanMode, "pwm")
	}
}

func TestGetInfo_NilProfile(t *testing.T) {
	ctrl := newProfiledController(t, nil)

	info := ctrl.GetInfo()

	// With nil profile, hardware fields should be zero values
	if info.Units != 0 {
		t.Errorf("Info.Units = %d, want 0 for nil profile", info.Units)
	}
}

func TestFactoryReset_WithProfile(t *testing.T) {
	p := hardware.MockProfile()
	ctrl := newProfiledController(t, p)
	ctx := context.Background()

	// Modify some state first
	name := "Custom Zone"
	ctrl.SetZone(ctx, 0, models.ZoneUpdate{Name: &name})

	// Reset
	state, appErr := ctrl.FactoryReset(ctx)
	if appErr != nil {
		t.Fatalf("FactoryReset: %v", appErr)
	}

	// State should be reset with correct zone count
	if len(state.Zones) != 6 {
		t.Errorf("Zones after reset = %d, want 6", len(state.Zones))
	}
}
