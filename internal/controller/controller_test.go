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

// memStore is an in-memory Store implementation for testing.
type memStore struct {
	state *models.State
}

func newMemStore() *memStore {
	def := models.DefaultState()
	return &memStore{state: &def}
}

func (m *memStore) Load() (*models.State, error) {
	cp := *m.state
	return &cp, nil
}

func (m *memStore) Save(s *models.State) error {
	cp := *s
	m.state = &cp
	return nil
}

func (m *memStore) Path() string { return ":memory:" }
func (m *memStore) Flush() error { return nil }

// Ensure memStore implements config.Store
var _ config.Store = (*memStore)(nil)

func newTestController(t *testing.T) *controller.Controller {
	t.Helper()
	hw := hardware.NewMock()
	store := newMemStore()
	bus := events.NewBus()
	ctrl, err := controller.New(hw, store, bus, nil)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}
	return ctrl
}

func TestControllerInitialState(t *testing.T) {
	ctrl := newTestController(t)
	state := ctrl.State()

	if len(state.Sources) != 4 {
		t.Errorf("expected 4 sources, got %d", len(state.Sources))
	}
	if len(state.Zones) != 6 {
		t.Errorf("expected 6 zones, got %d", len(state.Zones))
	}
}

func TestSetSource(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "My Source"
	state, appErr := ctrl.SetSource(ctx, 0, models.SourceUpdate{Name: &name})
	if appErr != nil {
		t.Fatalf("SetSource failed: %v", appErr)
	}
	if state.Sources[0].Name != name {
		t.Errorf("source name = %q, want %q", state.Sources[0].Name, name)
	}
}

func TestSetSourceInvalidID(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "Test"
	_, appErr := ctrl.SetSource(ctx, 99, models.SourceUpdate{Name: &name})
	if appErr == nil {
		t.Fatal("expected error for invalid source ID")
	}
	if appErr.Status != 400 {
		t.Errorf("expected status 400, got %d", appErr.Status)
	}
}

func TestSetZone(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -20
	mute := false
	state, appErr := ctrl.SetZone(ctx, 0, models.ZoneUpdate{Vol: &vol, Mute: &mute})
	if appErr != nil {
		t.Fatalf("SetZone failed: %v", appErr)
	}
	if state.Zones[0].Vol != -20 {
		t.Errorf("zone vol = %d, want -20", state.Zones[0].Vol)
	}
	if state.Zones[0].Mute {
		t.Error("zone should not be muted")
	}
}

func TestSetZoneInvalidID(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -20
	_, appErr := ctrl.SetZone(ctx, 999, models.ZoneUpdate{Vol: &vol})
	if appErr == nil {
		t.Fatal("expected error for invalid zone ID")
	}
}

func TestCreateAndDeleteGroup(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "Test Group"
	state, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0, 1},
	})
	if appErr != nil {
		t.Fatalf("CreateGroup failed: %v", appErr)
	}
	if len(state.Groups) == 0 {
		t.Fatal("expected at least one group after creation")
	}

	gid := state.Groups[len(state.Groups)-1].ID
	state, appErr = ctrl.DeleteGroup(ctx, gid)
	if appErr != nil {
		t.Fatalf("DeleteGroup failed: %v", appErr)
	}
	for _, g := range state.Groups {
		if g.ID == gid {
			t.Errorf("group %d still exists after delete", gid)
		}
	}
}

func TestCreatePreset(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	state, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{
		Name: "Test Preset",
	})
	if appErr != nil {
		t.Fatalf("CreatePreset failed: %v", appErr)
	}

	found := false
	for _, p := range state.Presets {
		if p.Name == "Test Preset" {
			found = true
			break
		}
	}
	if !found {
		t.Error("preset not found after creation")
	}
}

func TestFactoryReset(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Modify some state
	name := "Custom Zone"
	ctrl.SetZone(ctx, 0, models.ZoneUpdate{Name: &name})

	// Reset
	state, appErr := ctrl.FactoryReset(ctx)
	if appErr != nil {
		t.Fatalf("FactoryReset failed: %v", appErr)
	}
	if state.Zones[0].Name == "Custom Zone" {
		t.Error("factory reset did not restore default zone name")
	}
}
