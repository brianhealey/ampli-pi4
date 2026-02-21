package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// --- JSONStore tests ---

func newTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "amplipi-config-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestJSONStore_LoadMissingFile_ReturnsDefault(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if state == nil {
		t.Fatal("Load() returned nil state")
	}
	def := models.DefaultState()
	if len(state.Sources) != len(def.Sources) {
		t.Errorf("Load() sources = %d, want %d", len(state.Sources), len(def.Sources))
	}
	if len(state.Zones) != len(def.Zones) {
		t.Errorf("Load() zones = %d, want %d", len(state.Zones), len(def.Zones))
	}
}

func TestJSONStore_SaveLoadRoundTrip(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	st := models.DefaultState()
	st.Sources[0].Name = "Modified Source"
	st.Zones[0].Vol = -42

	if err := store.Save(&st); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	// Flush to ensure the file is written
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Sources[0].Name != "Modified Source" {
		t.Errorf("Sources[0].Name = %q, want %q", loaded.Sources[0].Name, "Modified Source")
	}
	if loaded.Zones[0].Vol != -42 {
		t.Errorf("Zones[0].Vol = %d, want -42", loaded.Zones[0].Vol)
	}
}

func TestJSONStore_CorruptJSON_ReturnsDefault(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Write corrupt JSON
	path := filepath.Join(dir, "house.json")
	if err := os.WriteFile(path, []byte("{invalid json!!!"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Should not panic or error — returns DefaultState
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if state == nil {
		t.Fatal("Load() returned nil state for corrupt JSON")
	}
	def := models.DefaultState()
	if len(state.Sources) != len(def.Sources) {
		t.Errorf("corrupt JSON: sources = %d, want %d (default)", len(state.Sources), len(def.Sources))
	}
}

func TestJSONStore_FlushAfterSave_FileExists(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	st := models.DefaultState()
	if err := store.Save(&st); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	path := store.Path()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist at %q after Flush, got: %v", path, err)
	}
}

func TestJSONStore_FlushWithoutSave_NoError(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Flush with nothing pending — should be a no-op, no error
	if err := store.Flush(); err != nil {
		t.Errorf("Flush() with no pending save: error = %v, want nil", err)
	}
}

func TestJSONStore_Path(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)
	p := store.Path()
	if p == "" {
		t.Error("Path() returned empty string")
	}
}

func TestJSONStore_MigratesMissingVolMinMax(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Write a JSON with zones that have vol_min=0 and vol_max=0 (missing field behavior)
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "Source 1", "input": ""},
			{"id": 1, "name": "Source 2", "input": ""},
			{"id": 2, "name": "Source 3", "input": ""},
			{"id": 3, "name": "Source 4", "input": ""},
		},
		"zones": []map[string]interface{}{
			{"id": 0, "name": "Zone 1", "source_id": 0, "mute": true, "vol": -80, "vol_f": 0.0},
			// vol_min and vol_max omitted → both will be 0 in Go zero values
		},
		"groups":  []interface{}{},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	path := filepath.Join(dir, "house.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(state.Zones) == 0 {
		t.Fatal("expected at least one zone")
	}

	z := state.Zones[0]
	if z.VolMin != models.MinVolDB {
		t.Errorf("after migration: VolMin = %d, want %d", z.VolMin, models.MinVolDB)
	}
	if z.VolMax != models.MaxVolDB {
		t.Errorf("after migration: VolMax = %d, want %d", z.VolMax, models.MaxVolDB)
	}
}

func TestJSONStore_MigratesInvalidZoneID(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Zone with invalid ID (-1)
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []map[string]interface{}{
			{"id": -1, "name": "Bad Zone", "source_id": 0, "mute": true, "vol": -80, "vol_f": 0.0, "vol_min": -80, "vol_max": 0},
		},
		"groups":  []interface{}{},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Zone ID should be fixed to index
	if len(state.Zones) > 0 && state.Zones[0].ID < 0 {
		t.Errorf("zone ID not fixed: got %d", state.Zones[0].ID)
	}
}

func TestJSONStore_MigratesVolClamped(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Zone with vol outside limits
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []map[string]interface{}{
			{"id": 0, "name": "Zone 1", "source_id": 0, "mute": true, "vol": 10, "vol_f": 1.0, "vol_min": -80, "vol_max": 0},
		},
		"groups":  []interface{}{},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Zones) > 0 && state.Zones[0].Vol > 0 {
		t.Errorf("vol not clamped: got %d", state.Zones[0].Vol)
	}
}

func TestJSONStore_MigratesNilSlices(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// No groups/streams/presets fields at all
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []map[string]interface{}{
			{"id": 0, "name": "Zone 1", "source_id": 0, "mute": true, "vol": -80, "vol_f": 0.0, "vol_min": -80, "vol_max": 0},
		},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// After migration, slices should not be nil
	if state.Groups == nil {
		t.Error("Groups should not be nil after migration")
	}
	if state.Streams == nil {
		t.Error("Streams should not be nil after migration")
	}
	if state.Presets == nil {
		t.Error("Presets should not be nil after migration")
	}
}

func TestJSONStore_MigratesFewerThan4Sources(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Only 2 sources in the file
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
		},
		"zones":   []interface{}{},
		"groups":  []interface{}{},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Should have been padded to at least 4 sources
	if len(state.Sources) < 4 {
		t.Errorf("sources = %d, want at least 4 after migration", len(state.Sources))
	}
}

// --- MemStore tests ---

func TestMemStore_SaveLoadRoundTrip(t *testing.T) {
	store := config.NewMemStore()

	st := models.DefaultState()
	st.Zones[2].Name = "Test Zone"
	st.Sources[1].Input = "local"

	if err := store.Save(&st); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Zones[2].Name != "Test Zone" {
		t.Errorf("Zones[2].Name = %q, want %q", loaded.Zones[2].Name, "Test Zone")
	}
	if loaded.Sources[1].Input != "local" {
		t.Errorf("Sources[1].Input = %q, want %q", loaded.Sources[1].Input, "local")
	}
}

func TestMemStore_LoadBeforeSave_ReturnsDefault(t *testing.T) {
	store := config.NewMemStore()

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	def := models.DefaultState()
	if len(state.Sources) != len(def.Sources) {
		t.Errorf("Load() sources = %d, want %d", len(state.Sources), len(def.Sources))
	}
	if len(state.Zones) != len(def.Zones) {
		t.Errorf("Load() zones = %d, want %d", len(state.Zones), len(def.Zones))
	}
}

func TestMemStore_MutationIsolation(t *testing.T) {
	store := config.NewMemStore()

	st := models.DefaultState()
	st.Zones[0].Vol = -30

	if err := store.Save(&st); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Mutate the returned value
	loaded.Zones[0].Vol = -99
	loaded.Sources[0].Name = "Mutated"

	// Load again — should still have original values
	loaded2, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded2.Zones[0].Vol != -30 {
		t.Errorf("isolation broken: Zones[0].Vol = %d, want -30", loaded2.Zones[0].Vol)
	}
	if loaded2.Sources[0].Name == "Mutated" {
		t.Error("isolation broken: Sources[0].Name was mutated through returned pointer")
	}
}

func TestMemStore_Path(t *testing.T) {
	store := config.NewMemStore()
	if store.Path() != ":memory:" {
		t.Errorf("Path() = %q, want \":memory:\"", store.Path())
	}
}

func TestMemStore_Flush_NoOp(t *testing.T) {
	store := config.NewMemStore()
	if err := store.Flush(); err != nil {
		t.Errorf("Flush() error = %v, want nil", err)
	}
}

func TestJSONStore_SaveTwice_StopsOldTimer(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	st1 := models.DefaultState()
	st1.Sources[0].Name = "First Save"

	st2 := models.DefaultState()
	st2.Sources[0].Name = "Second Save"

	// Call Save twice — second Save should stop the first timer and set a new one
	if err := store.Save(&st1); err != nil {
		t.Fatalf("First Save() error = %v", err)
	}
	if err := store.Save(&st2); err != nil {
		t.Fatalf("Second Save() error = %v", err)
	}
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	// The loaded state should reflect the second save
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Sources[0].Name != "Second Save" {
		t.Errorf("Sources[0].Name = %q, want %q", loaded.Sources[0].Name, "Second Save")
	}
}

func TestJSONStore_MigratesGroupWithNilZones(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Group with no zones field
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []interface{}{},
		"groups": []map[string]interface{}{
			{"id": 100, "name": "My Group"},
		},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Groups) == 0 {
		t.Fatal("Groups missing after migration")
	}
	// nil zone IDs should be replaced with empty slice
	if state.Groups[0].ZoneIDs == nil {
		t.Error("Group.ZoneIDs should not be nil after migration")
	}
}

func TestJSONStore_MigratesInvalidStreamID(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []interface{}{},
		"groups": []interface{}{},
		"streams": []map[string]interface{}{
			{"id": -5, "name": "Bad Stream", "type": "internet_radio"},
		},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Streams) > 0 && state.Streams[0].ID < 0 {
		t.Errorf("stream ID not fixed: got %d", state.Streams[0].ID)
	}
}

func TestJSONStore_MigratesInvalidPresetID(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []interface{}{},
		"groups": []interface{}{},
		"streams": []interface{}{},
		"presets": []map[string]interface{}{
			{"id": -3, "name": "Bad Preset"},
		},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Presets) > 0 && state.Presets[0].ID < 0 {
		t.Errorf("preset ID not fixed: got %d", state.Presets[0].ID)
	}
}

func TestJSONStore_MigratesInvalidGroupID(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []interface{}{},
		"groups": []map[string]interface{}{
			{"id": -5, "name": "Bad Group", "zones": []int{}},
		},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Groups) > 0 && state.Groups[0].ID < 0 {
		t.Errorf("group ID not fixed: got %d", state.Groups[0].ID)
	}
}

func TestJSONStore_MigratesZoneVolF(t *testing.T) {
	dir := newTempDir(t)
	store := config.NewJSONStore(dir)

	// Zone with vol=-40 but vol_f=0 (should get synced)
	raw := map[string]interface{}{
		"sources": []map[string]interface{}{
			{"id": 0, "name": "S1", "input": ""},
			{"id": 1, "name": "S2", "input": ""},
			{"id": 2, "name": "S3", "input": ""},
			{"id": 3, "name": "S4", "input": ""},
		},
		"zones": []map[string]interface{}{
			{"id": 0, "name": "Zone 1", "source_id": 0, "mute": false, "vol": -40, "vol_f": 0.0, "vol_min": -80, "vol_max": 0},
		},
		"groups":  []interface{}{},
		"streams": []interface{}{},
		"presets": []interface{}{},
	}
	data, _ := json.Marshal(raw)
	path := filepath.Join(dir, "house.json")
	os.WriteFile(path, data, 0644)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Zones) > 0 {
		// vol_f should be synced from vol=-40 → vol_f = 0.5
		if state.Zones[0].VolF < 0.4 || state.Zones[0].VolF > 0.6 {
			t.Errorf("vol_f not synced from vol=-40: got %f", state.Zones[0].VolF)
		}
	}
}

func TestMemStore_SaveMutationIsolation(t *testing.T) {
	store := config.NewMemStore()

	st := models.DefaultState()
	if err := store.Save(&st); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Mutate the original after saving
	st.Zones[0].Vol = -99

	// Store should still have the original value
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Zones[0].Vol == -99 {
		t.Error("Save did not deep copy: mutation of original affected stored state")
	}
}
