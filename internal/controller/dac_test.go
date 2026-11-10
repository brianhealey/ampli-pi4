package controller_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/controller"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// Digital stream IDs used by these tests.
const (
	airplayID = 1000
	spotifyID = 1001
	radioID   = 1002
)

// singleDACProfile is a first-generation unit: one main unit, only the
// HiFiBerry DAC (source 0), no USB DAC.
func singleDACProfile() *hardware.HardwareProfile {
	p := hardware.MockProfile()
	p.AvailablePhysicalOutputs = []int{0}
	return p
}

// stateWithDigitalStreams is the default state plus three digital streams.
func stateWithDigitalStreams() models.State {
	st := models.DefaultState()
	st.Streams = append(st.Streams,
		models.Stream{ID: airplayID, Name: "AirPlay", Type: models.StreamTypeAirPlay},
		models.Stream{ID: spotifyID, Name: "Spotify", Type: models.StreamTypeSpotify},
		models.Stream{ID: radioID, Name: "Radio", Type: models.StreamTypeInternetRadio,
			Config: map[string]interface{}{"url": "http://example.com/stream"}},
	)
	return st
}

func newDACController(t *testing.T, p *hardware.HardwareProfile, st models.State) (*controller.Controller, *config.MemStore) {
	t.Helper()
	store := config.NewMemStore()
	if err := store.Save(&st); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	ctrl, err := controller.New(hardware.NewMock(), p, store, events.NewBus(), nil)
	if err != nil {
		t.Fatalf("controller.New: %v", err)
	}
	t.Cleanup(ctrl.Shutdown)
	return ctrl, store
}

func input(id int) *string {
	s := "stream=" + strconv.Itoa(id)
	return &s
}

func sourceByID(t *testing.T, st models.State, id int) models.Source {
	t.Helper()
	for _, s := range st.Sources {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("source %d not found", id)
	return models.Source{}
}

func TestSetSource_DigitalOnSourceWithoutDACRejected(t *testing.T) {
	ctrl, _ := newDACController(t, singleDACProfile(), stateWithDigitalStreams())
	ctx := context.Background()

	for _, src := range []int{1, 2, 3} {
		_, appErr := ctrl.SetSource(ctx, src, models.SourceUpdate{Input: input(spotifyID)})
		if appErr == nil {
			t.Fatalf("source %d: digital stream accepted on a source without a DAC", src)
		}
		if appErr.Status != 400 {
			t.Errorf("source %d: status = %d, want 400", src, appErr.Status)
		}
		want := "source " + strconv.Itoa(src) + " has no digital output on this unit; digital streams can only play on source 0"
		if appErr.Message != want {
			t.Errorf("source %d: message = %q, want %q", src, appErr.Message, want)
		}
		if got := sourceByID(t, ctrl.State(), src).Input; got != "" {
			t.Errorf("source %d: input changed to %q after rejection", src, got)
		}
	}
}

func TestSetSource_AnalogOnSourceWithoutDACAccepted(t *testing.T) {
	ctrl, _ := newDACController(t, singleDACProfile(), stateWithDigitalStreams())
	ctx := context.Background()

	cases := map[int]string{
		1: "stream=997", // RCA "Input 2"
		2: "local",
		3: "stream=995", // Aux
	}
	for src, in := range cases {
		in := in
		st, appErr := ctrl.SetSource(ctx, src, models.SourceUpdate{Input: &in})
		if appErr != nil {
			t.Fatalf("source %d: analog input %q rejected: %v", src, in, appErr)
		}
		if got := sourceByID(t, st, src).Input; got != in {
			t.Errorf("source %d: input = %q, want %q", src, got, in)
		}
	}

	// Clearing the input is always allowed.
	empty := ""
	if _, appErr := ctrl.SetSource(ctx, 1, models.SourceUpdate{Input: &empty}); appErr != nil {
		t.Errorf("clearing source 1 rejected: %v", appErr)
	}
}

func TestSetSource_DigitalOnSourceZeroAccepted(t *testing.T) {
	ctrl, _ := newDACController(t, singleDACProfile(), stateWithDigitalStreams())

	st, appErr := ctrl.SetSource(context.Background(), 0, models.SourceUpdate{Input: input(airplayID)})
	if appErr != nil {
		t.Fatalf("digital stream on source 0 rejected: %v", appErr)
	}
	if got := sourceByID(t, st, 0).Input; got != "stream=1000" {
		t.Errorf("source 0 input = %q", got)
	}
}

func TestSetSource_FullDACProfileUnchanged(t *testing.T) {
	for name, p := range map[string]*hardware.HardwareProfile{
		"mock profile (4 DACs)": hardware.MockProfile(),
		"outputs not detected":  {Units: hardware.MockProfile().Units, TotalSources: 4},
		"nil profile":           nil,
	} {
		t.Run(name, func(t *testing.T) {
			ctrl, _ := newDACController(t, p, stateWithDigitalStreams())
			ctx := context.Background()
			for src, id := range map[int]int{0: airplayID, 1: spotifyID, 2: radioID, 3: airplayID} {
				if _, appErr := ctrl.SetSource(ctx, src, models.SourceUpdate{Input: input(id)}); appErr != nil {
					t.Errorf("source %d: digital stream rejected: %v", src, appErr)
				}
			}
			for _, s := range ctrl.State().Sources {
				if s.HasDAC != nil {
					t.Errorf("source %d: has_dac = %v, want omitted", s.ID, *s.HasDAC)
				}
			}
		})
	}
}

func TestHasDACExposedOnlyForSourcesWithoutDAC(t *testing.T) {
	ctrl, _ := newDACController(t, singleDACProfile(), stateWithDigitalStreams())

	data, err := json.Marshal(ctrl.State().Sources)
	if err != nil {
		t.Fatal(err)
	}
	var sources []map[string]interface{}
	if err := json.Unmarshal(data, &sources); err != nil {
		t.Fatal(err)
	}
	for _, s := range sources {
		id := int(s["id"].(float64))
		v, present := s["has_dac"]
		if id == 0 {
			if present {
				t.Errorf("source 0: has_dac present (%v), want omitted", v)
			}
			continue
		}
		if !present || v != false {
			t.Errorf("source %d: has_dac = %v (present=%v), want false", id, v, present)
		}
	}
}

func TestStartup_ClearsDigitalStreamsOnSourcesWithoutDAC(t *testing.T) {
	// Mirrors the live single-DAC unit: source 0 on RCA, digital streams on 1-3.
	st := stateWithDigitalStreams()
	st.Sources[0].Input = "stream=996"
	st.Sources[1].Input = "stream=1000"
	st.Sources[2].Input = "stream=1001"
	st.Sources[3].Input = "stream=1002"

	ctrl, store := newDACController(t, singleDACProfile(), st)

	want := map[int]string{0: "stream=996", 1: "", 2: "", 3: ""}
	for id, in := range want {
		if got := sourceByID(t, ctrl.State(), id).Input; got != in {
			t.Errorf("source %d: input = %q, want %q", id, got, in)
		}
	}

	persisted, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for id, in := range want {
		if got := sourceByID(t, *persisted, id).Input; got != in {
			t.Errorf("persisted source %d: input = %q, want %q", id, got, in)
		}
	}
}

func TestStartup_KeepsAnalogAndSourceZero(t *testing.T) {
	st := stateWithDigitalStreams()
	st.Sources[0].Input = "stream=1001" // digital on the DAC source: kept
	st.Sources[1].Input = "stream=997"  // RCA: kept
	st.Sources[2].Input = "local"       // analog: kept
	st.Sources[3].Input = "stream=999"  // RCA: kept

	ctrl, _ := newDACController(t, singleDACProfile(), st)
	for i, s := range st.Sources {
		if got := ctrl.State().Sources[i].Input; got != s.Input {
			t.Errorf("source %d: input = %q, want %q (unchanged)", s.ID, got, s.Input)
		}
	}
}

func TestStartup_FullDACProfileKeepsDigitalStreams(t *testing.T) {
	st := stateWithDigitalStreams()
	st.Sources[1].Input = "stream=1000"
	st.Sources[2].Input = "stream=1001"
	st.Sources[3].Input = "stream=1002"

	ctrl, _ := newDACController(t, hardware.MockProfile(), st)
	for i := 1; i <= 3; i++ {
		if got := ctrl.State().Sources[i].Input; got != st.Sources[i].Input {
			t.Errorf("source %d: input = %q, want %q", i, got, st.Sources[i].Input)
		}
	}
}

func TestLoadConfig_ClearsDigitalStreamsOnSourcesWithoutDAC(t *testing.T) {
	ctrl, _ := newDACController(t, singleDACProfile(), stateWithDigitalStreams())

	incoming := stateWithDigitalStreams()
	incoming.Sources[0].Input = "stream=1000"
	incoming.Sources[2].Input = "stream=1001"
	yes := true
	incoming.Sources[2].HasDAC = &yes // client-supplied value is ignored

	st, appErr := ctrl.LoadConfig(context.Background(), incoming)
	if appErr != nil {
		t.Fatalf("LoadConfig: %v", appErr)
	}
	if got := sourceByID(t, st, 0).Input; got != "stream=1000" {
		t.Errorf("source 0 input = %q", got)
	}
	src2 := sourceByID(t, st, 2)
	if src2.Input != "" {
		t.Errorf("source 2 input = %q, want cleared", src2.Input)
	}
	if src2.HasDAC == nil || *src2.HasDAC {
		t.Errorf("source 2 has_dac not re-derived from hardware")
	}
}

func TestLoadPreset_DigitalOnSourceWithoutDACRejected(t *testing.T) {
	ctrl, _ := newDACController(t, singleDACProfile(), stateWithDigitalStreams())
	ctx := context.Background()

	srcID := 2
	st, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{
		Name: "Kitchen Spotify",
		State: &models.PresetState{Sources: []models.SourceUpdate{
			{ID: &srcID, Input: input(spotifyID)},
		}},
	})
	if appErr != nil {
		t.Fatalf("CreatePreset: %v", appErr)
	}
	presetID := st.Presets[len(st.Presets)-1].ID

	_, appErr = ctrl.LoadPreset(ctx, presetID)
	if appErr == nil {
		t.Fatal("loading a preset that puts a digital stream on source 2 should fail")
	}
	if appErr.Status != 400 || !strings.Contains(appErr.Message, "source 2 has no digital output") {
		t.Errorf("unexpected error: %d %q", appErr.Status, appErr.Message)
	}
	if got := sourceByID(t, ctrl.State(), 2).Input; got != "" {
		t.Errorf("source 2 input = %q after rejected preset", got)
	}
}
