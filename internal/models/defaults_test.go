package models_test

import (
	"testing"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func TestDefaultState(t *testing.T) {
	s := models.DefaultState()

	if len(s.Sources) != 4 {
		t.Errorf("expected 4 sources, got %d", len(s.Sources))
	}
	if len(s.Zones) != 6 {
		t.Errorf("expected 6 zones, got %d", len(s.Zones))
	}
	if s.Groups == nil {
		t.Error("groups should not be nil")
	}

	// Check source IDs
	for i, src := range s.Sources {
		if src.ID != i {
			t.Errorf("source[%d].ID = %d, want %d", i, src.ID, i)
		}
		if src.Input != "" {
			t.Errorf("source[%d].Input = %q, want empty", i, src.Input)
		}
	}

	// Check zone defaults
	for i, z := range s.Zones {
		if z.ID != i {
			t.Errorf("zone[%d].ID = %d, want %d", i, z.ID, i)
		}
		if !z.Mute {
			t.Errorf("zone[%d].Mute = false, want true (default is muted)", i)
		}
		if z.Vol != models.MinVolDB {
			t.Errorf("zone[%d].Vol = %d, want %d", i, z.Vol, models.MinVolDB)
		}
		if z.VolMin != models.MinVolDB {
			t.Errorf("zone[%d].VolMin = %d, want %d", i, z.VolMin, models.MinVolDB)
		}
		if z.VolMax != models.MaxVolDB {
			t.Errorf("zone[%d].VolMax = %d, want %d", i, z.VolMax, models.MaxVolDB)
		}
	}
}

func TestDeepCopy(t *testing.T) {
	s := models.DefaultState()
	cp := s.DeepCopy()

	// Modifying the copy should not affect the original
	cp.Sources[0].Name = "Modified"
	if s.Sources[0].Name == "Modified" {
		t.Error("deep copy did not isolate Sources slice")
	}

	cp.Zones[0].Vol = -10
	if s.Zones[0].Vol == -10 {
		t.Error("deep copy did not isolate Zones slice")
	}
}

func TestVolConversions(t *testing.T) {
	tests := []struct {
		db   int
		volf float64
	}{
		{-80, 0.0},
		{0, 1.0},
		{-40, 0.5},
	}
	for _, tc := range tests {
		gotF := models.DBToVolF(tc.db)
		if abs(gotF-tc.volf) > 0.001 {
			t.Errorf("DBToVolF(%d) = %f, want %f", tc.db, gotF, tc.volf)
		}
		gotDB := models.VolFToDB(tc.volf)
		if gotDB != tc.db {
			t.Errorf("VolFToDB(%f) = %d, want %d", tc.volf, gotDB, tc.db)
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
