package models_test

import (
	"testing"

	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// twoUnitProfile is a main unit plus one expander — 12 zones of hardware.
func twoUnitProfile() *hardware.HardwareProfile {
	return &hardware.HardwareProfile{
		TotalSources: 4,
		TotalZones:   12,
		Units: []hardware.UnitInfo{
			{Index: 0, ZoneBase: 0, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
			{Index: 1, ZoneBase: 6, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeExpansion}},
		},
	}
}

// A config written when only one unit was present must gain the second unit's
// zones once that hardware appears. Without this, the expander is detected but
// its zones never reach the API or UI, and applyStateToHW leaves its amps
// disabled because the zones are absent from state.
func TestReconcileZonesAddsZonesForNewUnit(t *testing.T) {
	st := models.DefaultState() // 6 zones, hardware-unaware
	if len(st.Zones) != 6 {
		t.Fatalf("precondition: DefaultState has %d zones, want 6", len(st.Zones))
	}

	added := models.ReconcileZones(&st, twoUnitProfile())

	if added != 6 {
		t.Errorf("added = %d, want 6", added)
	}
	if len(st.Zones) != 12 {
		t.Fatalf("Zones = %d, want 12", len(st.Zones))
	}
	for i, z := range st.Zones {
		if z.ID != i {
			t.Errorf("Zones[%d].ID = %d, want %d", i, z.ID, i)
		}
	}
}

// New zones must arrive disabled so they cannot start driving speakers in
// rooms that are not wired up yet.
func TestReconcileZonesAddsNewZonesDisabledAndMuted(t *testing.T) {
	st := models.DefaultState()

	models.ReconcileZones(&st, twoUnitProfile())

	for _, z := range st.Zones[6:] {
		if !z.Disabled {
			t.Errorf("zone %d: Disabled = false, want true", z.ID)
		}
		if !z.Mute {
			t.Errorf("zone %d: Mute = false, want true", z.ID)
		}
	}
}

// Reconciliation must never clobber zones the user has already configured.
func TestReconcileZonesPreservesExistingZones(t *testing.T) {
	st := models.DefaultState()
	st.Zones[0].Name = "Master Bathroom"
	st.Zones[0].Vol = -32
	st.Zones[0].Mute = false
	st.Zones[0].Disabled = false

	models.ReconcileZones(&st, twoUnitProfile())

	got := st.Zones[0]
	if got.Name != "Master Bathroom" || got.Vol != -32 || got.Mute || got.Disabled {
		t.Errorf("existing zone was modified: %+v", got)
	}
}

// Running twice must not duplicate anything.
func TestReconcileZonesIsIdempotent(t *testing.T) {
	st := models.DefaultState()
	p := twoUnitProfile()

	models.ReconcileZones(&st, p)
	added := models.ReconcileZones(&st, p)

	if added != 0 {
		t.Errorf("second run added = %d, want 0", added)
	}
	if len(st.Zones) != 12 {
		t.Errorf("Zones = %d, want 12", len(st.Zones))
	}
}

// A nil profile means we could not read the hardware — leave state alone
// rather than inventing or removing zones.
func TestReconcileZonesNilProfileIsNoOp(t *testing.T) {
	st := models.DefaultState()

	added := models.ReconcileZones(&st, nil)

	if added != 0 || len(st.Zones) != 6 {
		t.Errorf("added = %d, zones = %d; want 0 and 6", added, len(st.Zones))
	}
}

// Unplugging an expander must not delete its configuration — the zones stay so
// the names and volumes survive until the hardware comes back.
func TestReconcileZonesKeepsZonesForAbsentHardware(t *testing.T) {
	st := models.DefaultState()
	models.ReconcileZones(&st, twoUnitProfile())
	st.Zones[6].Name = "Garage"

	singleUnit := &hardware.HardwareProfile{
		TotalSources: 4,
		TotalZones:   6,
		Units: []hardware.UnitInfo{
			{Index: 0, ZoneBase: 0, ZoneCount: 6, Board: hardware.BoardInfo{UnitType: hardware.UnitTypeMain}},
		},
	}
	models.ReconcileZones(&st, singleUnit)

	if len(st.Zones) != 12 {
		t.Fatalf("Zones = %d, want 12 (absent hardware must not drop config)", len(st.Zones))
	}
	if st.Zones[6].Name != "Garage" {
		t.Errorf("Zones[6].Name = %q, want %q", st.Zones[6].Name, "Garage")
	}
}
