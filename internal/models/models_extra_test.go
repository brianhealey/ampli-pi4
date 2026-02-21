package models_test

import (
	"encoding/json"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func TestAppError_JSON(t *testing.T) {
	appErr := models.ErrNotFound("resource not found")

	data, err := json.Marshal(appErr)
	if err != nil {
		t.Fatalf("json.Marshal(AppError): %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := m["error"]; !ok {
		t.Error("AppError JSON missing 'error' field")
	}
	if _, ok := m["message"]; !ok {
		t.Error("AppError JSON missing 'message' field")
	}
	// Status field should NOT be in JSON (tagged json:"-")
	if _, ok := m["status"]; ok {
		t.Error("AppError JSON should not contain 'status' field (json:\"-\")")
	}
}

func TestAppError_ErrorConstructors(t *testing.T) {
	tests := []struct {
		name   string
		err    *models.AppError
		status int
		code   string
	}{
		{"NotFound", models.ErrNotFound("not found"), 404, "NOT_FOUND"},
		{"BadRequest", models.ErrBadRequest("bad request"), 400, "BAD_REQUEST"},
		{"Internal", models.ErrInternal("internal error"), 500, "INTERNAL"},
		{"Conflict", models.ErrConflict("conflict"), 409, "CONFLICT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Status != tc.status {
				t.Errorf("%s.Status = %d, want %d", tc.name, tc.err.Status, tc.status)
			}
			if tc.err.Code != tc.code {
				t.Errorf("%s.Code = %q, want %q", tc.name, tc.err.Code, tc.code)
			}
			if tc.err.Error() == "" {
				t.Errorf("%s.Error() is empty", tc.name)
			}
		})
	}
}

func TestAppError_Unauthorized(t *testing.T) {
	if models.ErrUnauthorized.Status != 401 {
		t.Errorf("ErrUnauthorized.Status = %d, want 401", models.ErrUnauthorized.Status)
	}
	if models.ErrUnauthorized.Code != "UNAUTHORIZED" {
		t.Errorf("ErrUnauthorized.Code = %q, want UNAUTHORIZED", models.ErrUnauthorized.Code)
	}
}

func TestDeepCopyIsolation(t *testing.T) {
	original := models.DefaultState()
	original.Sources[0].Name = "Orig Source"
	original.Zones[0].Vol = -50
	original.Groups = append(original.Groups, models.Group{
		ID:      100,
		Name:    "Test Group",
		ZoneIDs: []int{0, 1, 2},
	})
	original.Streams = append(original.Streams, models.Stream{
		ID:     1000,
		Name:   "Test Stream",
		Config: map[string]interface{}{"url": "http://example.com"},
	})
	original.Presets = append(original.Presets, models.Preset{
		ID:   1,
		Name: "Test Preset",
	})

	cp := original.DeepCopy()

	// Mutate the copy
	cp.Sources[0].Name = "Modified Source"
	cp.Zones[0].Vol = -99
	cp.Groups[0].Name = "Modified Group"
	cp.Groups[0].ZoneIDs[0] = 99
	cp.Streams[len(cp.Streams)-1].Name = "Modified Stream"
	cp.Presets[len(cp.Presets)-1].Name = "Modified Preset"

	// Original should be unchanged
	if original.Sources[0].Name != "Orig Source" {
		t.Error("DeepCopy: Sources slice shared (name mutation leaked)")
	}
	if original.Zones[0].Vol != -50 {
		t.Error("DeepCopy: Zones slice shared (vol mutation leaked)")
	}
	if original.Groups[0].Name != "Test Group" {
		t.Error("DeepCopy: Groups slice shared (name mutation leaked)")
	}
	if original.Groups[0].ZoneIDs[0] != 0 {
		t.Error("DeepCopy: Groups ZoneIDs slice shared (element mutation leaked)")
	}
}

func TestDeepCopy_NilPointers(t *testing.T) {
	// A state with no optional fields set (nil pointers)
	s := models.State{
		Sources: []models.Source{{ID: 0}},
		Zones:   []models.Zone{{ID: 0}},
		Groups: []models.Group{
			{ID: 1, ZoneIDs: nil, SourceID: nil, Vol: nil, Mute: nil},
		},
		Streams: []models.Stream{
			{ID: 1000, Config: nil, Disabled: nil, Browsable: nil},
		},
		Presets: []models.Preset{
			{ID: 1, State: nil, Commands: nil},
		},
	}

	cp := s.DeepCopy()

	// Should not panic
	if len(cp.Sources) != 1 {
		t.Errorf("DeepCopy: Sources count = %d, want 1", len(cp.Sources))
	}
	if len(cp.Groups) != 1 {
		t.Errorf("DeepCopy: Groups count = %d, want 1", len(cp.Groups))
	}
}

func TestDefaultState_ZoneCount(t *testing.T) {
	s := models.DefaultState()
	if len(s.Zones) != 6 {
		t.Errorf("DefaultState has %d zones, want 6", len(s.Zones))
	}
}

func TestDefaultState_SourceCount(t *testing.T) {
	s := models.DefaultState()
	if len(s.Sources) != 4 {
		t.Errorf("DefaultState has %d sources, want 4", len(s.Sources))
	}
}

func TestDefaultState_Streams(t *testing.T) {
	s := models.DefaultState()
	if len(s.Streams) == 0 {
		t.Error("DefaultState has no streams")
	}
	// Should include Aux and 4 RCA streams
	var hasAux bool
	for _, st := range s.Streams {
		if st.ID == models.AuxStreamID {
			hasAux = true
		}
	}
	if !hasAux {
		t.Errorf("DefaultState missing Aux stream (ID=%d)", models.AuxStreamID)
	}
}

func TestDefaultState_MuteAllPreset(t *testing.T) {
	s := models.DefaultState()
	var found bool
	for _, p := range s.Presets {
		if p.ID == models.MuteAllPresetID {
			found = true
			if p.Name != "Mute All" {
				t.Errorf("MuteAll preset name = %q, want %q", p.Name, "Mute All")
			}
		}
	}
	if !found {
		t.Errorf("DefaultState missing MuteAll preset (ID=%d)", models.MuteAllPresetID)
	}
}

func TestClampVol_BoundaryValues(t *testing.T) {
	tests := []struct {
		vol    int
		min    int
		max    int
		expect int
	}{
		{-80, -80, 0, -80},  // At lower bound
		{0, -80, 0, 0},      // At upper bound
		{-40, -80, 0, -40},  // In range
		{-81, -80, 0, -80},  // Below min → clamp to min
		{1, -80, 0, 0},      // Above max → clamp to max
		{100, -80, 0, 0},    // Way above max
		{-200, -80, 0, -80}, // Way below min
		{-50, -60, -20, -50}, // Custom range, in range
		{-10, -60, -20, -20}, // Custom range, above max
		{-70, -60, -20, -60}, // Custom range, below min
	}
	for _, tc := range tests {
		got := models.ClampVol(tc.vol, tc.min, tc.max)
		if got != tc.expect {
			t.Errorf("ClampVol(%d, %d, %d) = %d, want %d", tc.vol, tc.min, tc.max, got, tc.expect)
		}
	}
}

func TestDBToVolF_Boundaries(t *testing.T) {
	tests := []struct {
		db   int
		want float64
	}{
		{-80, 0.0},
		{0, 1.0},
		{-40, 0.5},
		{-90, 0.0}, // below min → clamp
		{10, 1.0},  // above max → clamp
	}
	for _, tc := range tests {
		got := models.DBToVolF(tc.db)
		diff := got - tc.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.001 {
			t.Errorf("DBToVolF(%d) = %f, want %f", tc.db, got, tc.want)
		}
	}
}

func TestVolFToDB_Boundaries(t *testing.T) {
	tests := []struct {
		f    float64
		want int
	}{
		{0.0, -80},
		{1.0, 0},
		{0.5, -40},
		{-0.1, -80}, // below 0 → clamp
		{1.1, 0},    // above 1 → clamp
	}
	for _, tc := range tests {
		got := models.VolFToDB(tc.f)
		if got != tc.want {
			t.Errorf("VolFToDB(%f) = %d, want %d", tc.f, got, tc.want)
		}
	}
}

func TestDefaultState_ZoneDefaults(t *testing.T) {
	s := models.DefaultState()
	for i, z := range s.Zones {
		if z.VolMin != models.MinVolDB {
			t.Errorf("zone[%d].VolMin = %d, want %d", i, z.VolMin, models.MinVolDB)
		}
		if z.VolMax != models.MaxVolDB {
			t.Errorf("zone[%d].VolMax = %d, want %d", i, z.VolMax, models.MaxVolDB)
		}
		if !z.Mute {
			t.Errorf("zone[%d].Mute = false, want true (default muted)", i)
		}
	}
}

func TestDeepCopy_GroupPointers(t *testing.T) {
	trueVal := true
	falseVal := false
	v := 42
	vf := 0.5
	src := 1

	s := models.State{
		Groups: []models.Group{
			{
				ID:       1,
				ZoneIDs:  []int{0, 1},
				SourceID: &src,
				Vol:      &v,
				VolF:     &vf,
				Mute:     &trueVal,
			},
		},
	}

	cp := s.DeepCopy()

	// Mutate the copy's pointer values
	*cp.Groups[0].SourceID = 99
	*cp.Groups[0].Vol = 99
	*cp.Groups[0].VolF = 0.99
	*cp.Groups[0].Mute = false
	_ = falseVal // suppress unused warning

	// Original pointers should be unchanged
	if *s.Groups[0].SourceID != 1 {
		t.Error("DeepCopy: Group.SourceID pointer shared")
	}
	if *s.Groups[0].Vol != 42 {
		t.Error("DeepCopy: Group.Vol pointer shared")
	}
	if *s.Groups[0].VolF != 0.5 {
		t.Error("DeepCopy: Group.VolF pointer shared")
	}
	if !*s.Groups[0].Mute {
		t.Error("DeepCopy: Group.Mute pointer shared")
	}
}

func TestDeepCopy_StreamConfig(t *testing.T) {
	s := models.State{
		Streams: []models.Stream{
			{
				ID:   1000,
				Name: "Test",
				Config: map[string]interface{}{
					"url":  "http://example.com",
					"port": 8080,
				},
			},
		},
	}

	cp := s.DeepCopy()

	// Mutate the copy's config
	cp.Streams[0].Config["url"] = "http://modified.com"
	cp.Streams[0].Config["new_key"] = "new_value"

	// Original should be unchanged
	if s.Streams[0].Config["url"] != "http://example.com" {
		t.Error("DeepCopy: Stream.Config map shared")
	}
	if _, ok := s.Streams[0].Config["new_key"]; ok {
		t.Error("DeepCopy: Stream.Config map shared (new key leaked to original)")
	}
}

func TestDeepCopy_PresetState(t *testing.T) {
	id := 0
	mute := true
	s := models.State{
		Presets: []models.Preset{
			{
				ID:   1,
				Name: "Test",
				State: &models.PresetState{
					Zones: []models.ZoneUpdate{{ID: &id, Mute: &mute}},
				},
			},
		},
	}

	cp := s.DeepCopy()

	// Modify copy's preset state
	cp.Presets[0].Name = "Modified"

	if s.Presets[0].Name != "Test" {
		t.Error("DeepCopy: Preset name shared")
	}
}

func TestStreamConstants(t *testing.T) {
	// Verify stream type constants are non-empty
	types := []string{
		models.StreamTypePandora,
		models.StreamTypeAirPlay,
		models.StreamTypeSpotify,
		models.StreamTypeRCA,
		models.StreamTypeAux,
		models.StreamTypeInternetRadio,
	}
	for _, st := range types {
		if st == "" {
			t.Errorf("stream type constant is empty")
		}
	}

	// Verify stream ID constants are in expected range
	if models.AuxStreamID < 0 {
		t.Errorf("AuxStreamID = %d, should be positive", models.AuxStreamID)
	}
	for i, rca := range []int{models.RCAStream0, models.RCAStream1, models.RCAStream2, models.RCAStream3} {
		if rca <= 0 {
			t.Errorf("RCAStream%d = %d, should be positive", i, rca)
		}
	}
}
