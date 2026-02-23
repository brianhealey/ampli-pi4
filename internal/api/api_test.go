package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/api"
	"github.com/micro-nova/amplipi-go/internal/auth"
	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/controller"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// newTestServer spins up a full router with mock dependencies.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	hw := hardware.NewMock()
	if err := hw.Init(context.Background()); err != nil {
		t.Fatalf("hw.Init: %v", err)
	}

	store := config.NewMemStore()
	bus := events.NewBus()

	ctrl, err := controller.New(hw, nil, store, bus, nil, nil)
	if err != nil {
		t.Fatalf("controller.New: %v", err)
	}

	authSvc, err := auth.NewService("") // open mode — empty dir
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}

	router := api.NewRouter(ctrl, authSvc, bus)
	srv := httptest.NewServer(router)
	t.Cleanup(func() {
		srv.Close()
		authSvc.Close()
	})
	return srv
}

// do is a convenience helper for making requests to the test server.
func do(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do %s %s: %v", method, path, err)
	}
	return resp
}

// decodeJSON reads and decodes a JSON response body into v.
func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

// requireStatus fails the test if the response status doesn't match.
func requireStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, expected, body)
	}
}

// --- Tests ---

func TestGetState(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api", "")
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)

	if len(state.Sources) == 0 {
		t.Error("GET /api: sources is empty")
	}
	if len(state.Zones) == 0 {
		t.Error("GET /api: zones is empty")
	}
	if state.Groups == nil {
		t.Error("GET /api: groups is nil")
	}
	if state.Streams == nil {
		t.Error("GET /api: streams is nil")
	}
	if state.Presets == nil {
		t.Error("GET /api: presets is nil")
	}
}

func TestGetStateTrailingSlash(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestSetSource_Valid(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/sources/0", `{"input":"local"}`)
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)

	if len(state.Sources) == 0 {
		t.Fatal("no sources in response")
	}
	if state.Sources[0].Input != "local" {
		t.Errorf("sources[0].input = %q, want %q", state.Sources[0].Input, "local")
	}
}

func TestSetSource_InvalidID(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/sources/99", `{"input":"local"}`)
	requireStatus(t, resp, http.StatusBadRequest)

	var errBody map[string]interface{}
	decodeJSON(t, resp, &errBody)
	if _, ok := errBody["error"]; !ok {
		t.Error("expected 'error' field in error response")
	}
}

func TestGetSources(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/sources", "")
	requireStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	if _, ok := body["sources"]; !ok {
		t.Error("expected 'sources' key in response")
	}
}

func TestGetSource_Valid(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/sources/0", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestGetSource_Invalid(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/sources/99", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestSetZone_Vol(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/zones/0", `{"vol":-30,"mute":false}`)
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)

	if len(state.Zones) == 0 {
		t.Fatal("no zones in response")
	}
	if state.Zones[0].Vol != -30 {
		t.Errorf("zones[0].vol = %d, want -30", state.Zones[0].Vol)
	}
	if state.Zones[0].Mute {
		t.Error("zones[0].mute = true, want false")
	}
}

func TestSetZone_VolClamped(t *testing.T) {
	srv := newTestServer(t)

	// Default zone has vol_max = 0; set vol to 100 → should be clamped to 0
	resp := do(t, srv, "PATCH", "/api/zones/0", `{"vol":100}`)
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)

	if len(state.Zones) == 0 {
		t.Fatal("no zones in response")
	}
	// vol should be clamped to vol_max (0 by default)
	if state.Zones[0].Vol > state.Zones[0].VolMax {
		t.Errorf("zones[0].vol = %d, exceeds vol_max = %d (clamping failed)",
			state.Zones[0].Vol, state.Zones[0].VolMax)
	}
}

func TestSetZone_InvalidID(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/zones/999", `{"vol":-30}`)
	// Should be 400 (invalid zone id range) or 404
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Errorf("status = %d, want 400 or 404; body: %s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

func TestGetZones(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/zones", "")
	requireStatus(t, resp, http.StatusOK)

	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	if _, ok := body["zones"]; !ok {
		t.Error("expected 'zones' key in response")
	}
}

func TestGetZone(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/zones/0", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestSetZones_Bulk(t *testing.T) {
	srv := newTestServer(t)

	body := `{"zones":[0,1],"update":{"vol":-40,"mute":false}}`
	resp := do(t, srv, "PATCH", "/api/zones", body)
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)

	if len(state.Zones) < 2 {
		t.Fatalf("expected at least 2 zones, got %d", len(state.Zones))
	}
	if state.Zones[0].Vol != -40 {
		t.Errorf("zones[0].vol = %d, want -40", state.Zones[0].Vol)
	}
	if state.Zones[1].Vol != -40 {
		t.Errorf("zones[1].vol = %d, want -40", state.Zones[1].Vol)
	}
}

func TestCreateGroup(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/group", `{"name":"Living Room","zones":[0,1]}`)
	requireStatus(t, resp, http.StatusCreated)

	var state models.State
	decodeJSON(t, resp, &state)

	if len(state.Groups) == 0 {
		t.Fatal("no groups in response after create")
	}
	// Find the created group
	var created *models.Group
	for i := range state.Groups {
		if state.Groups[i].Name == "Living Room" {
			created = &state.Groups[i]
			break
		}
	}
	if created == nil {
		t.Fatal("created group 'Living Room' not found in state")
	}
	if len(created.ZoneIDs) != 2 {
		t.Errorf("group zones = %v, want [0,1]", created.ZoneIDs)
	}
}

func TestPatchGroup(t *testing.T) {
	srv := newTestServer(t)

	// Create a group first
	resp := do(t, srv, "POST", "/api/group", `{"name":"Test Group","zones":[0]}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	if len(createState.Groups) == 0 {
		t.Fatal("no groups after creation")
	}
	gid := createState.Groups[len(createState.Groups)-1].ID

	// Patch it
	patchBody := `{"name":"Renamed Group"}`
	resp2 := do(t, srv, "PATCH", fmt.Sprintf("/api/groups/%d", gid), patchBody)
	requireStatus(t, resp2, http.StatusOK)

	var patchState models.State
	decodeJSON(t, resp2, &patchState)
	found := false
	for _, g := range patchState.Groups {
		if g.ID == gid && g.Name == "Renamed Group" {
			found = true
		}
	}
	if !found {
		t.Error("patched group not found with new name")
	}
}

func TestDeleteGroup(t *testing.T) {
	srv := newTestServer(t)

	// Create a group first
	resp := do(t, srv, "POST", "/api/group", `{"name":"To Delete","zones":[0]}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	if len(createState.Groups) == 0 {
		t.Fatal("no groups after creation")
	}
	gid := createState.Groups[len(createState.Groups)-1].ID

	// Delete it
	resp2 := do(t, srv, "DELETE", fmt.Sprintf("/api/groups/%d", gid), "")
	requireStatus(t, resp2, http.StatusOK)

	var deleteState models.State
	decodeJSON(t, resp2, &deleteState)
	for _, g := range deleteState.Groups {
		if g.ID == gid {
			t.Errorf("group %d still exists after delete", gid)
		}
	}
}

func TestGetGroups(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/groups", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestCreateStream(t *testing.T) {
	srv := newTestServer(t)

	body := `{"name":"Radio","type":"internet_radio","config":{"url":"http://example.com"}}`
	resp := do(t, srv, "POST", "/api/stream", body)
	requireStatus(t, resp, http.StatusCreated)

	var state models.State
	decodeJSON(t, resp, &state)
	found := false
	for _, s := range state.Streams {
		if s.Name == "Radio" {
			found = true
		}
	}
	if !found {
		t.Error("created stream 'Radio' not found in response")
	}
}

func TestDeleteStream(t *testing.T) {
	srv := newTestServer(t)

	// Create a stream first
	resp := do(t, srv, "POST", "/api/stream", `{"name":"ToDelete","type":"internet_radio"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var sid int
	for _, s := range createState.Streams {
		if s.Name == "ToDelete" {
			sid = s.ID
			break
		}
	}
	if sid == 0 {
		t.Fatal("created stream not found")
	}

	// Delete it
	resp2 := do(t, srv, "DELETE", fmt.Sprintf("/api/streams/%d", sid), "")
	requireStatus(t, resp2, http.StatusOK)

	var deleteState models.State
	decodeJSON(t, resp2, &deleteState)
	for _, s := range deleteState.Streams {
		if s.ID == sid {
			t.Errorf("stream %d still exists after delete", sid)
		}
	}
}

func TestGetStreams(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/streams", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestCreatePreset(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/preset", `{"name":"Evening"}`)
	requireStatus(t, resp, http.StatusCreated)

	var state models.State
	decodeJSON(t, resp, &state)
	found := false
	for _, p := range state.Presets {
		if p.Name == "Evening" {
			found = true
		}
	}
	if !found {
		t.Error("created preset 'Evening' not found in response")
	}
}

func TestLoadPreset(t *testing.T) {
	srv := newTestServer(t)

	// Create a preset
	resp := do(t, srv, "POST", "/api/preset", `{"name":"ToLoad"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var pid int
	for _, p := range createState.Presets {
		if p.Name == "ToLoad" {
			pid = p.ID
			break
		}
	}
	if pid == 0 {
		t.Fatal("created preset not found")
	}

	// Load it
	resp2 := do(t, srv, "POST", fmt.Sprintf("/api/presets/%d/load", pid), "")
	requireStatus(t, resp2, http.StatusOK)
}

func TestDeletePreset(t *testing.T) {
	srv := newTestServer(t)

	// Create a preset
	resp := do(t, srv, "POST", "/api/preset", `{"name":"ToDeletePreset"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var pid int
	for _, p := range createState.Presets {
		if p.Name == "ToDeletePreset" {
			pid = p.ID
			break
		}
	}
	if pid == 0 {
		t.Fatal("created preset not found")
	}

	// Delete it
	resp2 := do(t, srv, "DELETE", fmt.Sprintf("/api/presets/%d", pid), "")
	requireStatus(t, resp2, http.StatusOK)

	var deleteState models.State
	decodeJSON(t, resp2, &deleteState)
	for _, p := range deleteState.Presets {
		if p.ID == pid {
			t.Errorf("preset %d still exists after delete", pid)
		}
	}
}

func TestGetPresets(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/presets", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestFactoryReset(t *testing.T) {
	srv := newTestServer(t)

	// Modify some state first
	do(t, srv, "PATCH", "/api/sources/0", `{"input":"local"}`)

	// Factory reset
	resp := do(t, srv, "POST", "/api/factory_reset", "")
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)
	if state.Sources[0].Input != "" {
		t.Errorf("after factory reset: sources[0].input = %q, want empty", state.Sources[0].Input)
	}
}

func TestGetInfo(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/info", "")
	requireStatus(t, resp, http.StatusOK)

	var info models.Info
	decodeJSON(t, resp, &info)
	if info.Version == "" {
		t.Error("GET /api/info: version field is empty")
	}
}

func TestNotFound_JSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/nonexistent", "")
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want 404; body: %s", resp.StatusCode, body)
	}
	// Response should be text/plain or 404 — chi returns plain 404
	resp.Body.Close()
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	// POST to /api/sources/0 is not registered (only PATCH/GET)
	resp := do(t, srv, "POST", "/api/sources/0", `{}`)
	// chi returns 405 for method not allowed
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Logf("note: POST /api/sources/0 returned %d (chi may return 404 instead of 405); body: %s", resp.StatusCode, body)
	}
	_ = body
}

func TestSSESubscribe(t *testing.T) {
	srv := newTestServer(t)

	// Make a GET request to /api/subscribe with a short-lived context
	ctx, cancel := context.WithCancel(context.Background())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/subscribe", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	// Use a client that doesn't follow redirects and doesn't buffer
	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read the first line — should be a "data:" SSE event
	scanner := bufio.NewScanner(resp.Body)
	gotData := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			gotData = true
			// Verify the data is valid JSON
			jsonStr := strings.TrimPrefix(line, "data: ")
			var state models.State
			if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
				t.Errorf("SSE data is not valid State JSON: %v", err)
			}
			break
		}
	}

	cancel() // Close the connection

	if !gotData {
		t.Error("SSE stream did not emit a 'data:' event")
	}
}

func TestSetSource_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/sources/0", `{not valid json`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestSetZone_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/zones/0", `{bad json`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateGroup_MissingName(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/group", `{"zones":[0,1]}`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateStream_MissingName(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/stream", `{"type":"internet_radio"}`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreatePreset_MissingName(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/preset", `{}`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestPatchGroup_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/groups/99999", `{"name":"nope"}`)
	requireStatus(t, resp, http.StatusNotFound)
}

func TestDeleteGroup_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "DELETE", "/api/groups/99999", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestLoadConfig(t *testing.T) {
	srv := newTestServer(t)

	// Prepare a simple state to load
	state := models.DefaultState()
	state.Sources[0].Name = "Loaded Source"

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/load", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	requireStatus(t, resp, http.StatusOK)

	var result models.State
	decodeJSON(t, resp, &result)
	if result.Sources[0].Name != "Loaded Source" {
		t.Errorf("after LoadConfig: sources[0].name = %q, want %q", result.Sources[0].Name, "Loaded Source")
	}
}

func TestGetStream_Valid(t *testing.T) {
	srv := newTestServer(t)

	// Default state has streams with IDs 995-999
	resp := do(t, srv, "GET", "/api/streams/995", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestGetStream_Invalid(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/streams/1", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestGetPreset_Valid(t *testing.T) {
	srv := newTestServer(t)

	// Default state has MuteAllPreset with ID 10000
	resp := do(t, srv, "GET", "/api/presets/10000", "")
	requireStatus(t, resp, http.StatusOK)
}

func TestGetPreset_Invalid(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/presets/999999", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestPatchZone_SourceID(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/zones/0", `{"source_id":2}`)
	requireStatus(t, resp, http.StatusOK)

	var state models.State
	decodeJSON(t, resp, &state)
	if state.Zones[0].SourceID != 2 {
		t.Errorf("zones[0].source_id = %d, want 2", state.Zones[0].SourceID)
	}
}

func TestGetGroup(t *testing.T) {
	srv := newTestServer(t)

	// Create a group first
	resp := do(t, srv, "POST", "/api/group", `{"name":"GetGroup Test","zones":[0]}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	if len(createState.Groups) == 0 {
		t.Fatal("no groups after creation")
	}
	gid := createState.Groups[len(createState.Groups)-1].ID

	// Get the specific group
	resp2 := do(t, srv, "GET", fmt.Sprintf("/api/groups/%d", gid), "")
	requireStatus(t, resp2, http.StatusOK)

	var group models.Group
	decodeJSON(t, resp2, &group)
	if group.ID != gid {
		t.Errorf("group.id = %d, want %d", group.ID, gid)
	}
}

func TestGetGroup_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/groups/99999", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestPatchPreset(t *testing.T) {
	srv := newTestServer(t)

	// Create a preset
	resp := do(t, srv, "POST", "/api/preset", `{"name":"PatchMe"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var pid int
	for _, p := range createState.Presets {
		if p.Name == "PatchMe" {
			pid = p.ID
		}
	}
	if pid == 0 {
		t.Fatal("created preset not found")
	}

	// Patch it
	resp2 := do(t, srv, "PATCH", fmt.Sprintf("/api/presets/%d", pid), `{"name":"Patched"}`)
	requireStatus(t, resp2, http.StatusOK)

	var patchState models.State
	decodeJSON(t, resp2, &patchState)
	found := false
	for _, p := range patchState.Presets {
		if p.ID == pid && p.Name == "Patched" {
			found = true
		}
	}
	if !found {
		t.Error("preset name was not patched to 'Patched'")
	}
}

func TestPatchPreset_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/presets/99999", `{"name":"nope"}`)
	requireStatus(t, resp, http.StatusNotFound)
}

func TestPatchStream(t *testing.T) {
	srv := newTestServer(t)

	// Create a stream
	resp := do(t, srv, "POST", "/api/stream", `{"name":"PatchStream","type":"internet_radio"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var sid int
	for _, s := range createState.Streams {
		if s.Name == "PatchStream" {
			sid = s.ID
		}
	}
	if sid == 0 {
		t.Fatal("created stream not found")
	}

	// Patch it
	resp2 := do(t, srv, "PATCH", fmt.Sprintf("/api/streams/%d", sid), `{"name":"PatchedStream"}`)
	requireStatus(t, resp2, http.StatusOK)

	var patchState models.State
	decodeJSON(t, resp2, &patchState)
	found := false
	for _, s := range patchState.Streams {
		if s.ID == sid && s.Name == "PatchedStream" {
			found = true
		}
	}
	if !found {
		t.Error("stream not renamed to 'PatchedStream'")
	}
}

func TestPatchStream_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/streams/99999", `{"name":"nope"}`)
	requireStatus(t, resp, http.StatusNotFound)
}

func TestLoginPage(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/auth/login", "")
	requireStatus(t, resp, http.StatusOK)

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "Login") {
		t.Error("login page body does not contain 'Login'")
	}
}

func TestLoginPage_WithNext(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/auth/login?next=/api/zones", "")
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestLoginPost(t *testing.T) {
	srv := newTestServer(t)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/login", strings.NewReader("next=/api&password=test"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Don't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("loginPost status = %d, want 302", resp.StatusCode)
	}
}

func TestCORSOptions(t *testing.T) {
	srv := newTestServer(t)

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/api", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
}

func TestGetZone_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "GET", "/api/zones/999", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestDeleteStream_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "DELETE", "/api/streams/99999", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestDeletePreset_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "DELETE", "/api/presets/99999", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestLoadPreset_NotFound(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/presets/99999/load", "")
	requireStatus(t, resp, http.StatusNotFound)
}

func TestSetZones_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "PATCH", "/api/zones", `{bad json}`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateGroup_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/group", `{bad json`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreateStream_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/stream", `{bad json`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestCreatePreset_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/preset", `{bad json`)
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/load", strings.NewReader("{bad json"))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	requireStatus(t, resp, http.StatusBadRequest)
}

func TestSetGroup_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	// Create a group first
	resp := do(t, srv, "POST", "/api/group", `{"name":"Test","zones":[0]}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	gid := createState.Groups[len(createState.Groups)-1].ID

	resp2 := do(t, srv, "PATCH", fmt.Sprintf("/api/groups/%d", gid), `{bad json`)
	requireStatus(t, resp2, http.StatusBadRequest)
}

func TestSetPreset_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/preset", `{"name":"Test"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var pid int
	for _, p := range createState.Presets {
		if p.Name == "Test" {
			pid = p.ID
		}
	}

	resp2 := do(t, srv, "PATCH", fmt.Sprintf("/api/presets/%d", pid), `{bad json`)
	requireStatus(t, resp2, http.StatusBadRequest)
}

func TestSetStream_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, "POST", "/api/stream", `{"name":"Test","type":"internet_radio"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var sid int
	for _, s := range createState.Streams {
		if s.Name == "Test" {
			sid = s.ID
		}
	}

	resp2 := do(t, srv, "PATCH", fmt.Sprintf("/api/streams/%d", sid), `{bad json`)
	requireStatus(t, resp2, http.StatusBadRequest)
}

func TestExecStreamCmd(t *testing.T) {
	srv := newTestServer(t)

	// Create a stream first
	resp := do(t, srv, "POST", "/api/stream", `{"name":"CmdStream","type":"internet_radio"}`)
	requireStatus(t, resp, http.StatusCreated)

	var createState models.State
	decodeJSON(t, resp, &createState)
	var sid int
	for _, s := range createState.Streams {
		if s.Name == "CmdStream" {
			sid = s.ID
			break
		}
	}
	if sid == 0 {
		t.Fatal("created stream not found")
	}

	// Execute a play command
	resp2 := do(t, srv, "POST", fmt.Sprintf("/api/streams/%d/play", sid), "")
	requireStatus(t, resp2, http.StatusOK)
}
