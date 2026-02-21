package controller_test

import (
	"context"
	"sync"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func TestSetZoneVolClamped_AboveMax(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Default zone VolMax = 0. Setting vol to 100 should clamp to 0.
	vol := 100
	state, appErr := ctrl.SetZone(ctx, 0, models.ZoneUpdate{Vol: &vol})
	if appErr != nil {
		t.Fatalf("SetZone: %v", appErr)
	}

	if state.Zones[0].Vol != 0 {
		t.Errorf("vol after clamp = %d, want 0 (vol_max)", state.Zones[0].Vol)
	}
	if state.Zones[0].Vol > state.Zones[0].VolMax {
		t.Errorf("vol %d exceeds vol_max %d", state.Zones[0].Vol, state.Zones[0].VolMax)
	}
}

func TestSetZoneVolClamped_BelowMin(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Default zone VolMin = -80. Setting vol to -200 should clamp to -80.
	vol := -200
	state, appErr := ctrl.SetZone(ctx, 0, models.ZoneUpdate{Vol: &vol})
	if appErr != nil {
		t.Fatalf("SetZone: %v", appErr)
	}

	if state.Zones[0].Vol != -80 {
		t.Errorf("vol after clamp = %d, want -80 (vol_min)", state.Zones[0].Vol)
	}
	if state.Zones[0].Vol < state.Zones[0].VolMin {
		t.Errorf("vol %d is below vol_min %d", state.Zones[0].Vol, state.Zones[0].VolMin)
	}
}

func TestSetZoneVol_Exact(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -40
	state, appErr := ctrl.SetZone(ctx, 0, models.ZoneUpdate{Vol: &vol})
	if appErr != nil {
		t.Fatalf("SetZone: %v", appErr)
	}
	if state.Zones[0].Vol != -40 {
		t.Errorf("vol = %d, want -40", state.Zones[0].Vol)
	}
}

func TestSetZoneVol_VolF_Propagates(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -40 // should give vol_f = 0.5 (halfway between -80 and 0)
	state, appErr := ctrl.SetZone(ctx, 0, models.ZoneUpdate{Vol: &vol})
	if appErr != nil {
		t.Fatalf("SetZone: %v", appErr)
	}

	// VolF should be approximately 0.5
	if state.Zones[0].VolF < 0.49 || state.Zones[0].VolF > 0.51 {
		t.Errorf("vol_f = %f, want ~0.5 for vol=-40", state.Zones[0].VolF)
	}
}

func TestGroupVolPropagates(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Set a known starting volume for zones 0 and 1
	startVol := -60
	ctrl.SetZone(ctx, 0, models.ZoneUpdate{Vol: &startVol})
	ctrl.SetZone(ctx, 1, models.ZoneUpdate{Vol: &startVol})

	// Create a group with zones [0, 1]
	name := "Vol Test Group"
	createState, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0, 1},
	})
	if appErr != nil {
		t.Fatalf("CreateGroup: %v", appErr)
	}

	gid := createState.Groups[len(createState.Groups)-1].ID

	// Set group vol delta = +10 (relative)
	delta := 10
	patchState, appErr := ctrl.SetGroup(ctx, gid, models.GroupUpdate{Vol: &delta})
	if appErr != nil {
		t.Fatalf("SetGroup: %v", appErr)
	}

	// Both zones should have moved from -60 by +10 = -50, clamped to vol_max(0)
	expectedVol := -50
	zone0 := patchState.Zones[0]
	zone1 := patchState.Zones[1]

	if zone0.Vol != expectedVol {
		t.Errorf("after group vol update: zone0.vol = %d, want %d", zone0.Vol, expectedVol)
	}
	if zone1.Vol != expectedVol {
		t.Errorf("after group vol update: zone1.vol = %d, want %d", zone1.Vol, expectedVol)
	}
}

func TestGroupMutePropagates(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Set zones 0 and 1 to unmuted
	mute := false
	ctrl.SetZone(ctx, 0, models.ZoneUpdate{Mute: &mute})
	ctrl.SetZone(ctx, 1, models.ZoneUpdate{Mute: &mute})

	// Create a group with zones [0, 1]
	name := "Mute Test Group"
	createState, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0, 1},
	})
	if appErr != nil {
		t.Fatalf("CreateGroup: %v", appErr)
	}

	gid := createState.Groups[len(createState.Groups)-1].ID

	// Set group mute = true
	groupMute := true
	patchState, appErr := ctrl.SetGroup(ctx, gid, models.GroupUpdate{Mute: &groupMute})
	if appErr != nil {
		t.Fatalf("SetGroup with mute: %v", appErr)
	}

	// Both zones should now be muted
	if !patchState.Zones[0].Mute {
		t.Error("zone 0 should be muted after group mute")
	}
	if !patchState.Zones[1].Mute {
		t.Error("zone 1 should be muted after group mute")
	}
}

func TestGroupMuteUnmute(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Create a group with zones [0, 1]
	name := "Unmute Group"
	createState, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0, 1},
	})
	if appErr != nil {
		t.Fatalf("CreateGroup: %v", appErr)
	}

	gid := createState.Groups[len(createState.Groups)-1].ID

	// Mute then unmute
	muteTrue := true
	ctrl.SetGroup(ctx, gid, models.GroupUpdate{Mute: &muteTrue})

	muteFalse := false
	patchState, appErr := ctrl.SetGroup(ctx, gid, models.GroupUpdate{Mute: &muteFalse})
	if appErr != nil {
		t.Fatalf("SetGroup unmute: %v", appErr)
	}

	if patchState.Zones[0].Mute {
		t.Error("zone 0 should be unmuted after group unmute")
	}
	if patchState.Zones[1].Mute {
		t.Error("zone 1 should be unmuted after group unmute")
	}
}

func TestConcurrentSetZone(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Run 50 goroutines each calling SetZone on different zones
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			zoneID := i % 6 // only 6 zones exist
			vol := -(i % 80)
			ctrl.SetZone(ctx, zoneID, models.ZoneUpdate{Vol: &vol})
		}(i)
	}

	wg.Wait()

	// Final state should be consistent (no panic, no data race)
	state := ctrl.State()
	for _, z := range state.Zones {
		if z.Vol < models.MinVolDB || z.Vol > models.MaxVolDB {
			t.Errorf("zone %d vol = %d, out of range [%d, %d]", z.ID, z.Vol, models.MinVolDB, models.MaxVolDB)
		}
	}
}

func TestConcurrentSetSource(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	inputs := []string{"local", "", "stream=1000"}
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			srcID := i % 4
			input := inputs[i%len(inputs)]
			ctrl.SetSource(ctx, srcID, models.SourceUpdate{Input: &input})
		}(i)
	}

	wg.Wait()

	// State should be consistent
	state := ctrl.State()
	if len(state.Sources) != 4 {
		t.Errorf("sources count = %d, want 4 after concurrent updates", len(state.Sources))
	}
}

func TestSetZones_BulkInvalidZone(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -30
	_, appErr := ctrl.SetZones(ctx, models.MultiZoneUpdate{
		ZoneIDs: []int{0, 9999},
		Update:  models.ZoneUpdate{Vol: &vol},
	})
	if appErr == nil {
		t.Error("SetZones with invalid zone ID should return error")
	}
}

func TestGetZone_NotFound(t *testing.T) {
	ctrl := newTestController(t)

	_, appErr := ctrl.GetZone(9999)
	if appErr == nil {
		t.Error("GetZone(9999) should return error")
	}
	if appErr.Status != 404 {
		t.Errorf("GetZone not found status = %d, want 404", appErr.Status)
	}
}

func TestGetSource_NotFound(t *testing.T) {
	ctrl := newTestController(t)

	_, appErr := ctrl.GetSource(99)
	if appErr == nil {
		t.Error("GetSource(99) should return error")
	}
}

func TestGetGroup_NotFound(t *testing.T) {
	ctrl := newTestController(t)

	_, appErr := ctrl.GetGroup(9999)
	if appErr == nil {
		t.Error("GetGroup(9999) should return error")
	}
}

func TestGetStream_NotFound(t *testing.T) {
	ctrl := newTestController(t)

	_, appErr := ctrl.GetStream(9999)
	if appErr == nil {
		t.Error("GetStream(9999) should return error")
	}
}

func TestGetPreset_NotFound(t *testing.T) {
	ctrl := newTestController(t)

	_, appErr := ctrl.GetPreset(9999)
	if appErr == nil {
		t.Error("GetPreset(9999) should return error")
	}
}

func TestCreateStream_MissingName(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Type: "internet_radio"})
	if appErr == nil {
		t.Error("CreateStream with no name should return error")
	}
}

func TestCreateStream_MissingType(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "Test"})
	if appErr == nil {
		t.Error("CreateStream with no type should return error")
	}
}

func TestCreateGroup_MissingName(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{ZoneIDs: []int{0}})
	if appErr == nil {
		t.Error("CreateGroup with no name should return error")
	}
}

func TestCreatePreset_MissingName(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{})
	if appErr == nil {
		t.Error("CreatePreset with no name should return error")
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.DeleteGroup(ctx, 9999)
	if appErr == nil {
		t.Error("DeleteGroup(9999) should return error")
	}
}

func TestDeleteStream_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.DeleteStream(ctx, 9999)
	if appErr == nil {
		t.Error("DeleteStream(9999) should return error")
	}
}

func TestDeletePreset_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.DeletePreset(ctx, 9999)
	if appErr == nil {
		t.Error("DeletePreset(9999) should return error")
	}
}

func TestSetPreset(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	createState, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{Name: "Original"})
	if appErr != nil {
		t.Fatalf("CreatePreset: %v", appErr)
	}

	var pid int
	for _, p := range createState.Presets {
		if p.Name == "Original" {
			pid = p.ID
		}
	}

	newName := "Updated"
	patchState, appErr := ctrl.SetPreset(ctx, pid, models.PresetUpdate{Name: &newName})
	if appErr != nil {
		t.Fatalf("SetPreset: %v", appErr)
	}

	found := false
	for _, p := range patchState.Presets {
		if p.ID == pid && p.Name == "Updated" {
			found = true
		}
	}
	if !found {
		t.Error("preset not updated to 'Updated' name")
	}
}

func TestSetStream(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	createState, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "MyStream", Type: "internet_radio"})
	if appErr != nil {
		t.Fatalf("CreateStream: %v", appErr)
	}

	var sid int
	for _, s := range createState.Streams {
		if s.Name == "MyStream" {
			sid = s.ID
		}
	}

	newName := "RenamedStream"
	patchState, appErr := ctrl.SetStream(ctx, sid, models.StreamUpdate{Name: &newName})
	if appErr != nil {
		t.Fatalf("SetStream: %v", appErr)
	}

	found := false
	for _, s := range patchState.Streams {
		if s.ID == sid && s.Name == "RenamedStream" {
			found = true
		}
	}
	if !found {
		t.Error("stream not renamed to 'RenamedStream'")
	}
}

func TestExecStreamCommand_Play(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	createState, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "PlayStream", Type: "internet_radio"})
	if appErr != nil {
		t.Fatalf("CreateStream: %v", appErr)
	}

	var sid int
	for _, s := range createState.Streams {
		if s.Name == "PlayStream" {
			sid = s.ID
		}
	}

	cmdState, appErr := ctrl.ExecStreamCommand(ctx, sid, "play")
	if appErr != nil {
		t.Fatalf("ExecStreamCommand: %v", appErr)
	}

	for _, s := range cmdState.Streams {
		if s.ID == sid && s.Info.State == "playing" {
			return // success
		}
	}
	t.Error("stream state is not 'playing' after play command")
}

func TestLoadPreset_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.LoadPreset(ctx, 9999)
	if appErr == nil {
		t.Error("LoadPreset(9999) should return error")
	}
}

func TestGetInfo(t *testing.T) {
	ctrl := newTestController(t)

	info := ctrl.GetInfo()
	if info.Version == "" {
		t.Error("GetInfo().Version is empty")
	}
}

func TestSetZone_Name(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "Master Bedroom"
	state, appErr := ctrl.SetZone(ctx, 1, models.ZoneUpdate{Name: &name})
	if appErr != nil {
		t.Fatalf("SetZone: %v", appErr)
	}

	if state.Zones[1].Name != "Master Bedroom" {
		t.Errorf("zone 1 name = %q, want %q", state.Zones[1].Name, "Master Bedroom")
	}
}

func TestSetZone_Disabled(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	disabled := true
	state, appErr := ctrl.SetZone(ctx, 2, models.ZoneUpdate{Disabled: &disabled})
	if appErr != nil {
		t.Fatalf("SetZone: %v", appErr)
	}

	if !state.Zones[2].Disabled {
		t.Error("zone 2 should be disabled")
	}
}

func TestGroupSourcePropagates(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "Source Group"
	createState, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0, 1},
	})
	if appErr != nil {
		t.Fatalf("CreateGroup: %v", appErr)
	}

	gid := createState.Groups[len(createState.Groups)-1].ID

	sourceID := 2
	patchState, appErr := ctrl.SetGroup(ctx, gid, models.GroupUpdate{SourceID: &sourceID})
	if appErr != nil {
		t.Fatalf("SetGroup: %v", appErr)
	}

	if patchState.Zones[0].SourceID != 2 {
		t.Errorf("zone 0 source_id = %d, want 2 after group source update", patchState.Zones[0].SourceID)
	}
	if patchState.Zones[1].SourceID != 2 {
		t.Errorf("zone 1 source_id = %d, want 2 after group source update", patchState.Zones[1].SourceID)
	}
}

func TestLoadConfig(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	incoming := models.DefaultState()
	incoming.Sources[0].Name = "Custom Source"

	state, appErr := ctrl.LoadConfig(ctx, incoming)
	if appErr != nil {
		t.Fatalf("LoadConfig: %v", appErr)
	}

	if state.Sources[0].Name != "Custom Source" {
		t.Errorf("after LoadConfig: sources[0].name = %q, want %q", state.Sources[0].Name, "Custom Source")
	}
}

func TestGetSources(t *testing.T) {
	ctrl := newTestController(t)
	sources := ctrl.GetSources()
	if len(sources) != 4 {
		t.Errorf("GetSources() returned %d sources, want 4", len(sources))
	}
}

func TestGetZones(t *testing.T) {
	ctrl := newTestController(t)
	zones := ctrl.GetZones()
	if len(zones) != 6 {
		t.Errorf("GetZones() returned %d zones, want 6", len(zones))
	}
}

func TestGetGroups(t *testing.T) {
	ctrl := newTestController(t)
	groups := ctrl.GetGroups()
	// Default state has no groups
	if groups == nil {
		t.Error("GetGroups() returned nil, want empty slice")
	}
}

func TestGetStreams(t *testing.T) {
	ctrl := newTestController(t)
	streams := ctrl.GetStreams()
	// Default state has Aux + 4 RCA streams
	if len(streams) == 0 {
		t.Error("GetStreams() returned empty slice")
	}
}

func TestGetPresets(t *testing.T) {
	ctrl := newTestController(t)
	presets := ctrl.GetPresets()
	// Default state has MuteAll preset
	if len(presets) == 0 {
		t.Error("GetPresets() returned empty slice")
	}
}

func TestSetZones_BulkValid(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -35
	state, appErr := ctrl.SetZones(ctx, models.MultiZoneUpdate{
		ZoneIDs: []int{0, 1, 2},
		Update:  models.ZoneUpdate{Vol: &vol},
	})
	if appErr != nil {
		t.Fatalf("SetZones: %v", appErr)
	}

	// All 3 zones should have vol = -35
	for _, zid := range []int{0, 1, 2} {
		if state.Zones[zid].Vol != -35 {
			t.Errorf("zone %d vol = %d, want -35", zid, state.Zones[zid].Vol)
		}
	}
}

func TestSetZones_EmptyZoneIDs(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	vol := -35
	state, appErr := ctrl.SetZones(ctx, models.MultiZoneUpdate{
		ZoneIDs: []int{},
		Update:  models.ZoneUpdate{Vol: &vol},
	})
	if appErr != nil {
		t.Fatalf("SetZones with empty zones: %v", appErr)
	}
	_ = state
}

func TestLoadPreset_WithState(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Create a preset with some zone updates
	zoneID := 0
	vol := -50
	mute := false
	createState, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{
		Name: "Vol Preset",
		State: &models.PresetState{
			Zones: []models.ZoneUpdate{
				{ID: &zoneID, Vol: &vol, Mute: &mute},
			},
		},
	})
	if appErr != nil {
		t.Fatalf("CreatePreset: %v", appErr)
	}

	var pid int
	for _, p := range createState.Presets {
		if p.Name == "Vol Preset" {
			pid = p.ID
		}
	}

	// Load the preset
	loadedState, appErr := ctrl.LoadPreset(ctx, pid)
	if appErr != nil {
		t.Fatalf("LoadPreset: %v", appErr)
	}

	// Zone 0 should have vol = -50
	if loadedState.Zones[0].Vol != -50 {
		t.Errorf("after LoadPreset: zones[0].vol = %d, want -50", loadedState.Zones[0].Vol)
	}
}

func TestLoadPreset_MuteAll(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// First unmute zone 0
	mute := false
	ctrl.SetZone(ctx, 0, models.ZoneUpdate{Mute: &mute})

	// Load the MuteAll preset (ID 10000)
	state, appErr := ctrl.LoadPreset(ctx, models.MuteAllPresetID)
	if appErr != nil {
		t.Fatalf("LoadPreset(MuteAll): %v", appErr)
	}

	// All zones should be muted
	for _, z := range state.Zones {
		if !z.Mute {
			t.Errorf("zone %d not muted after MuteAll preset", z.ID)
		}
	}
}

func TestSetGroup_VolF(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "VolF Group"
	createState, appErr := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0, 1},
	})
	if appErr != nil {
		t.Fatalf("CreateGroup: %v", appErr)
	}
	gid := createState.Groups[len(createState.Groups)-1].ID

	// Set vol_f = 0.5 → should set zones to about -40 dB
	volF := 0.5
	patchState, appErr := ctrl.SetGroup(ctx, gid, models.GroupUpdate{VolF: &volF})
	if appErr != nil {
		t.Fatalf("SetGroup with VolF: %v", appErr)
	}

	// Zone 0 vol_f should be approximately 0.5
	if patchState.Zones[0].VolF < 0.4 || patchState.Zones[0].VolF > 0.6 {
		t.Errorf("zone 0 vol_f = %f, want ~0.5", patchState.Zones[0].VolF)
	}
}

func TestSetGroup_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "Ghost"
	_, appErr := ctrl.SetGroup(ctx, 99999, models.GroupUpdate{Name: &name})
	if appErr == nil {
		t.Error("SetGroup with invalid ID should return error")
	}
}

func TestSetStream_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "New Name"
	_, appErr := ctrl.SetStream(ctx, 99999, models.StreamUpdate{Name: &name})
	if appErr == nil {
		t.Error("SetStream with invalid ID should return error")
	}
}

func TestSetPreset_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "New Name"
	_, appErr := ctrl.SetPreset(ctx, 99999, models.PresetUpdate{Name: &name})
	if appErr == nil {
		t.Error("SetPreset with invalid ID should return error")
	}
}

func TestExecStreamCommand_Pause(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	createState, appErr := ctrl.CreateStream(ctx, models.StreamCreate{Name: "PauseStream", Type: "internet_radio"})
	if appErr != nil {
		t.Fatalf("CreateStream: %v", appErr)
	}

	var sid int
	for _, s := range createState.Streams {
		if s.Name == "PauseStream" {
			sid = s.ID
		}
	}

	cmdState, appErr := ctrl.ExecStreamCommand(ctx, sid, "pause")
	if appErr != nil {
		t.Fatalf("ExecStreamCommand(pause): %v", appErr)
	}

	for _, s := range cmdState.Streams {
		if s.ID == sid && s.Info.State == "paused" {
			return
		}
	}
	t.Error("stream state is not 'paused' after pause command")
}

func TestExecStreamCommand_Stop(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	createState, _ := ctrl.CreateStream(ctx, models.StreamCreate{Name: "StopStream", Type: "internet_radio"})
	var sid int
	for _, s := range createState.Streams {
		if s.Name == "StopStream" {
			sid = s.ID
		}
	}

	cmdState, appErr := ctrl.ExecStreamCommand(ctx, sid, "stop")
	if appErr != nil {
		t.Fatalf("ExecStreamCommand(stop): %v", appErr)
	}

	for _, s := range cmdState.Streams {
		if s.ID == sid && s.Info.State == "stopped" {
			return
		}
	}
	t.Error("stream state is not 'stopped' after stop command")
}

func TestExecStreamCommand_Unknown(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	createState, _ := ctrl.CreateStream(ctx, models.StreamCreate{Name: "CmdStream", Type: "internet_radio"})
	var sid int
	for _, s := range createState.Streams {
		if s.Name == "CmdStream" {
			sid = s.ID
		}
	}

	// Unknown command should not fail
	_, appErr := ctrl.ExecStreamCommand(ctx, sid, "unknown-cmd")
	if appErr != nil {
		t.Errorf("ExecStreamCommand with unknown cmd should not fail: %v", appErr)
	}
}

func TestExecStreamCommand_NotFound(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	_, appErr := ctrl.ExecStreamCommand(ctx, 99999, "play")
	if appErr == nil {
		t.Error("ExecStreamCommand with invalid ID should return error")
	}
}

func TestIsAnalogInput(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Set source 0 to "local" (analog)
	input := "local"
	state, appErr := ctrl.SetSource(ctx, 0, models.SourceUpdate{Input: &input})
	if appErr != nil {
		t.Fatalf("SetSource: %v", appErr)
	}
	if state.Sources[0].Input != "local" {
		t.Errorf("source 0 input = %q, want %q", state.Sources[0].Input, "local")
	}

	// Set source 1 to a digital stream
	digitalInput := "stream=1000"
	state, appErr = ctrl.SetSource(ctx, 1, models.SourceUpdate{Input: &digitalInput})
	if appErr != nil {
		t.Fatalf("SetSource: %v", appErr)
	}
	if state.Sources[1].Input != "stream=1000" {
		t.Errorf("source 1 input = %q, want %q", state.Sources[1].Input, "stream=1000")
	}
}

func TestSetSource_Name(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "My Speaker"
	state, appErr := ctrl.SetSource(ctx, 2, models.SourceUpdate{Name: &name})
	if appErr != nil {
		t.Fatalf("SetSource: %v", appErr)
	}
	if state.Sources[2].Name != "My Speaker" {
		t.Errorf("source 2 name = %q, want %q", state.Sources[2].Name, "My Speaker")
	}
}

func TestSetZone_VolDeltaF(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	// Start from default vol = -80
	// Apply delta_f = 0.25 → vol delta in range = 0.25 * 80 = 20, new vol = -80 + 20 = -60
	delta := 0.25
	state, appErr := ctrl.SetZone(ctx, 0, models.ZoneUpdate{VolDeltaF: &delta})
	if appErr != nil {
		t.Fatalf("SetZone with VolDeltaF: %v", appErr)
	}
	if state.Zones[0].Vol != -60 {
		t.Errorf("zone 0 vol after VolDeltaF(0.25) = %d, want -60", state.Zones[0].Vol)
	}
}

func TestSetGroup_ZoneIDs(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	name := "Zone Update Group"
	createState, _ := ctrl.CreateGroup(ctx, models.GroupUpdate{
		Name:    &name,
		ZoneIDs: []int{0},
	})
	gid := createState.Groups[len(createState.Groups)-1].ID

	// Update zone IDs
	patchState, appErr := ctrl.SetGroup(ctx, gid, models.GroupUpdate{ZoneIDs: []int{0, 1, 2}})
	if appErr != nil {
		t.Fatalf("SetGroup ZoneIDs: %v", appErr)
	}

	for _, g := range patchState.Groups {
		if g.ID == gid {
			if len(g.ZoneIDs) != 3 {
				t.Errorf("group zones = %v, want [0,1,2]", g.ZoneIDs)
			}
			return
		}
	}
	t.Error("group not found after zone ID update")
}

func TestLoadPreset_WithSources(t *testing.T) {
	ctrl := newTestController(t)
	ctx := context.Background()

	srcID := 0
	newInput := "local"
	createState, appErr := ctrl.CreatePreset(ctx, models.PresetCreate{
		Name: "Source Preset",
		State: &models.PresetState{
			Sources: []models.SourceUpdate{
				{ID: &srcID, Input: &newInput},
			},
		},
	})
	if appErr != nil {
		t.Fatalf("CreatePreset: %v", appErr)
	}

	var pid int
	for _, p := range createState.Presets {
		if p.Name == "Source Preset" {
			pid = p.ID
		}
	}

	loadedState, appErr := ctrl.LoadPreset(ctx, pid)
	if appErr != nil {
		t.Fatalf("LoadPreset: %v", appErr)
	}
	if loadedState.Sources[0].Input != "local" {
		t.Errorf("after source preset: sources[0].input = %q, want local", loadedState.Sources[0].Input)
	}
}
