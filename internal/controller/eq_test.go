package controller_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/controller"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

func sampleEQ() *models.SourceEQ {
	return &models.SourceEQ{
		Enabled:  true,
		PreampDB: -6,
		Bands: []models.EQBand{
			{Type: "lowshelf", Freq: 100, Gain: 6, Q: 0.7},
			{Type: "peaking", Freq: 1000, Gain: -3, Q: 2},
		},
	}
}

func TestSetSource_EQValidation(t *testing.T) {
	ctrl, _ := newDACController(t, hardware.MockProfile(), stateWithDigitalStreams())
	ctx := context.Background()

	tooMany := sampleEQ()
	for len(tooMany.Bands) <= models.EQMaxBands {
		tooMany.Bands = append(tooMany.Bands, models.EQBand{Type: "peaking", Freq: 500, Gain: 1, Q: 1})
	}
	cases := []struct {
		name string
		mut  func(*models.SourceEQ)
		want string
	}{
		{"preamp too high", func(e *models.SourceEQ) { e.PreampDB = 1 }, "eq.preamp_db must be between -24 and 0 dB"},
		{"preamp too low", func(e *models.SourceEQ) { e.PreampDB = -25 }, "eq.preamp_db must be between -24 and 0 dB"},
		{"too many bands", func(e *models.SourceEQ) { *e = *tooMany }, "eq.bands: at most 10 bands are allowed, got 11"},
		{"bad type", func(e *models.SourceEQ) { e.Bands[1].Type = "notch" }, `eq.bands[1].type must be one of "peaking", "lowshelf", "highshelf", got "notch"`},
		{"freq low", func(e *models.SourceEQ) { e.Bands[0].Freq = 19 }, "eq.bands[0].freq must be between 20 and 20000 Hz"},
		{"freq high", func(e *models.SourceEQ) { e.Bands[0].Freq = 20001 }, "eq.bands[0].freq must be between 20 and 20000 Hz"},
		{"gain high", func(e *models.SourceEQ) { e.Bands[1].Gain = 12.5 }, "eq.bands[1].gain must be between -12 and 12 dB"},
		{"gain low", func(e *models.SourceEQ) { e.Bands[1].Gain = -13 }, "eq.bands[1].gain must be between -12 and 12 dB"},
		{"q low", func(e *models.SourceEQ) { e.Bands[0].Q = 0.05 }, "eq.bands[0].q must be between 0.1 and 10"},
		{"q high", func(e *models.SourceEQ) { e.Bands[0].Q = 11 }, "eq.bands[0].q must be between 0.1 and 10"},
	}
	for _, tc := range cases {
		eq := sampleEQ()
		tc.mut(eq)
		_, appErr := ctrl.SetSource(ctx, 0, models.SourceUpdate{EQ: eq})
		if appErr == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if appErr.Status != 400 || appErr.Message != tc.want {
			t.Errorf("%s: got %d %q, want 400 %q", tc.name, appErr.Status, appErr.Message, tc.want)
		}
	}
	if sourceByID(t, ctrl.State(), 0).EQ != nil {
		t.Error("rejected EQ was stored")
	}

	// Boundaries are inclusive; zero bands is fine.
	edge := &models.SourceEQ{Enabled: true, PreampDB: -24, Bands: []models.EQBand{
		{Type: "highshelf", Freq: 20000, Gain: 12, Q: 10},
		{Type: "peaking", Freq: 20, Gain: -12, Q: 0.1},
	}}
	for _, eq := range []*models.SourceEQ{edge, {Enabled: true}} {
		if _, appErr := ctrl.SetSource(ctx, 0, models.SourceUpdate{EQ: eq}); appErr != nil {
			t.Errorf("valid EQ %+v rejected: %v", eq, appErr)
		}
	}
}

func TestSetSource_EQPatchSemantics(t *testing.T) {
	ctrl, _ := newDACController(t, hardware.MockProfile(), stateWithDigitalStreams())
	ctx := context.Background()

	if _, appErr := ctrl.SetSource(ctx, 1, models.SourceUpdate{EQ: sampleEQ()}); appErr != nil {
		t.Fatal(appErr)
	}
	// Omitted eq: unchanged, while other fields still apply.
	name := "Kitchen"
	st, appErr := ctrl.SetSource(ctx, 1, models.SourceUpdate{Name: &name, Input: input(spotifyID)})
	if appErr != nil {
		t.Fatal(appErr)
	}
	src := sourceByID(t, st, 1)
	if src.Name != "Kitchen" || src.Input != "stream=1001" {
		t.Errorf("name/input not applied: %+v", src)
	}
	if got, _ := json.Marshal(src.EQ); string(got) != mustJSON(t, sampleEQ()) {
		t.Errorf("eq changed by a PATCH without eq: %s", got)
	}

	// eq present: replaced as a whole.
	off := &models.SourceEQ{Enabled: false, PreampDB: -6, Bands: sampleEQ().Bands[:1]}
	st, _ = ctrl.SetSource(ctx, 1, models.SourceUpdate{EQ: off})
	if e := sourceByID(t, st, 1).EQ; e == nil || e.Enabled || len(e.Bands) != 1 {
		t.Errorf("eq not replaced: %+v", e)
	}

	// The stored EQ must not alias the request body.
	off.Bands[0].Gain = 9
	if g := sourceByID(t, ctrl.State(), 1).EQ.Bands[0].Gain; g != 6 {
		t.Errorf("stored EQ aliases the request: gain %v", g)
	}

	// Raw JSON PATCH without eq leaves it alone (pointer field, not zeroed).
	var upd models.SourceUpdate
	if err := json.Unmarshal([]byte(`{"name":"Den"}`), &upd); err != nil {
		t.Fatal(err)
	}
	st, _ = ctrl.SetSource(ctx, 1, upd)
	if sourceByID(t, st, 1).EQ == nil {
		t.Error(`{"name":...} cleared the eq`)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSourceEQ_PersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := config.NewJSONStore(dir)
	ctrl, err := controller.New(hardware.NewMock(), hardware.MockProfile(), store, events.NewBus(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, appErr := ctrl.SetSource(context.Background(), 2, models.SourceUpdate{EQ: sampleEQ()}); appErr != nil {
		t.Fatal(appErr)
	}
	ctrl.Shutdown()
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"preamp_db": -6`) {
		t.Errorf("house.json lacks the eq:\n%s", raw)
	}
	if strings.Contains(string(raw), "eq_status") || strings.Contains(string(raw), "eq_available") {
		t.Errorf("derived EQ status was persisted:\n%s", raw)
	}

	ctrl2, err := controller.New(hardware.NewMock(), hardware.MockProfile(), config.NewJSONStore(dir), events.NewBus(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl2.Shutdown)
	got := sourceByID(t, ctrl2.State(), 2).EQ
	if mustJSON(t, got) != mustJSON(t, sampleEQ()) {
		t.Errorf("reloaded eq = %s, want %s", mustJSON(t, got), mustJSON(t, sampleEQ()))
	}
	for _, id := range []int{0, 1, 3} {
		if sourceByID(t, ctrl2.State(), id).EQ != nil {
			t.Errorf("source %d gained an eq", id)
		}
	}
}

func TestSourceEQ_InvalidStoredEQDropped(t *testing.T) {
	st := stateWithDigitalStreams()
	bad := sampleEQ()
	bad.Bands[0].Freq = 5 // hand-edited house.json
	st.Sources[0].EQ = bad
	st.Sources[1].EQ = sampleEQ()
	st.Sources[1].EQStatus = &models.EQStatus{State: "active"} // must never come from storage
	ctrl, store := newDACController(t, hardware.MockProfile(), st)

	if sourceByID(t, ctrl.State(), 0).EQ != nil {
		t.Error("invalid stored eq kept")
	}
	if sourceByID(t, ctrl.State(), 1).EQ == nil {
		t.Error("valid stored eq dropped")
	}
	saved, _ := store.Load()
	if saved.Sources[1].EQStatus != nil {
		t.Error("stored eq_status not cleared")
	}
}

func TestSourceEQ_Status(t *testing.T) {
	noEQ := hardware.MockProfile()
	noEQ.EQBinary = ""
	ctx := context.Background()

	type step struct {
		profile *hardware.HardwareProfile
		src     int
		input   string
		enabled bool
		want    string
	}
	steps := []step{
		{hardware.MockProfile(), 0, "stream=1000", false, models.EQStateOff},
		{noEQ, 0, "stream=1000", true, models.EQStateUnavailable},
		{singleDACProfile(), 2, "local", true, models.EQStateNoDAC},
		{hardware.MockProfile(), 0, "", true, models.EQStateIdle},
		{hardware.MockProfile(), 1, "stream=997", true, models.EQStateAnalog}, // RCA 2
		{hardware.MockProfile(), 3, "stream=995", true, models.EQStateAnalog}, // Aux
		{hardware.MockProfile(), 0, "local", true, models.EQStateAnalog},
		// No stream manager in these tests.
		{hardware.MockProfile(), 0, "stream=1000", true, models.EQStateIdle},
	}
	for i, s := range steps {
		ctrl, _ := newDACController(t, s.profile, stateWithDigitalStreams())
		in := s.input
		eq := sampleEQ()
		eq.Enabled = s.enabled
		st, appErr := ctrl.SetSource(ctx, s.src, models.SourceUpdate{Input: &in, EQ: eq})
		if appErr != nil {
			t.Fatalf("step %d: %v", i, appErr)
		}
		for name, got := range map[string]models.State{"apply result": st, "State()": ctrl.State()} {
			es := sourceByID(t, got, s.src).EQStatus
			if es == nil || es.State != s.want {
				t.Errorf("step %d (%s): eq_status = %+v, want %q", i, name, es, s.want)
			}
			if s.want != models.EQStateOff && s.want != models.EQStateIdle && es != nil && es.Reason == "" {
				t.Errorf("step %d: %q without a reason", i, s.want)
			}
		}
		src, _ := ctrl.GetSource(s.src)
		if src.EQStatus == nil || src.EQStatus.State != s.want {
			t.Errorf("step %d: GetSource eq_status = %+v", i, src.EQStatus)
		}
		if want := s.profile.EQAvailable(); ctrl.State().Info.EQAvailable != want || ctrl.GetInfo().EQAvailable != want {
			t.Errorf("step %d: eq_available != %v", i, want)
		}
	}
}

// A source with no EQ configured must look exactly as before the feature.
func TestSourceEQ_NoEQNoStatus(t *testing.T) {
	ctrl, _ := newDACController(t, hardware.MockProfile(), stateWithDigitalStreams())
	st, _ := ctrl.SetSource(context.Background(), 0, models.SourceUpdate{Input: input(airplayID)})
	b, _ := json.Marshal(sourceByID(t, st, 0))
	if strings.Contains(string(b), `"eq`) {
		t.Errorf("source without eq serialises eq fields: %s", b)
	}
}

func TestLoadPreset_AppliesValidatedEQ(t *testing.T) {
	ctrl, _ := newDACController(t, hardware.MockProfile(), stateWithDigitalStreams())
	ctx := context.Background()
	id := 0
	bad := sampleEQ()
	bad.PreampDB = 3

	st, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{Name: "eq", State: &models.PresetState{
		Sources: []models.SourceUpdate{{ID: &id, EQ: sampleEQ()}}}})
	if appErr != nil {
		t.Fatal(appErr)
	}
	pid := st.Presets[len(st.Presets)-1].ID
	if _, appErr := ctrl.LoadPreset(ctx, pid); appErr != nil {
		t.Fatal(appErr)
	}
	if sourceByID(t, ctrl.State(), 0).EQ == nil {
		t.Error("preset eq not applied")
	}

	st, _ = ctrl.CreatePreset(ctx, models.PresetCreate{Name: "bad", State: &models.PresetState{
		Sources: []models.SourceUpdate{{ID: &id, EQ: bad}}}})
	if _, appErr := ctrl.LoadPreset(ctx, st.Presets[len(st.Presets)-1].ID); appErr == nil || appErr.Status != 400 {
		t.Errorf("invalid preset eq: got %v, want 400", appErr)
	}
}
