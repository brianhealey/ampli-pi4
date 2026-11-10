package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func TestSetSource_EQThroughAPI(t *testing.T) {
	srv := newTestServer(t)

	body := `{"eq":{"enabled":true,"preamp_db":-6,"bands":[` +
		`{"type":"lowshelf","freq":100,"gain":6,"q":0.7},` +
		`{"type":"peaking","freq":3000,"gain":-2,"q":1.4}]}}`
	resp := do(t, srv, "PATCH", "/api/sources/0", body)
	requireStatus(t, resp, http.StatusOK)
	var state models.State
	decodeJSON(t, resp, &state)
	eq := state.Sources[0].EQ
	if eq == nil || !eq.Enabled || eq.PreampDB != -6 || len(eq.Bands) != 2 || eq.Bands[1].Freq != 3000 {
		t.Fatalf("eq not stored: %+v", eq)
	}
	// The test server has no hardware profile, so CamillaDSP counts as missing:
	// the setting is stored but reported unavailable.
	if st := state.Sources[0].EQStatus; st == nil || st.State != models.EQStateUnavailable {
		t.Errorf("eq_status = %+v, want unavailable", st)
	}

	// A PATCH without eq leaves it alone.
	resp = do(t, srv, "PATCH", "/api/sources/0", `{"name":"Living room"}`)
	requireStatus(t, resp, http.StatusOK)
	resp = do(t, srv, "GET", "/api/sources/0", "")
	requireStatus(t, resp, http.StatusOK)
	var src models.Source
	decodeJSON(t, resp, &src)
	if src.Name != "Living room" || src.EQ == nil || len(src.EQ.Bands) != 2 {
		t.Errorf("source after name-only PATCH: %+v", src)
	}

	// Validation errors are 400s with a message naming the field.
	resp = do(t, srv, "PATCH", "/api/sources/0", `{"eq":{"enabled":true,"preamp_db":0,"bands":[{"type":"peaking","freq":1000,"gain":20,"q":1}]}}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	msg, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(msg), "eq.bands[0].gain must be between -12 and 12 dB") {
		t.Errorf("error body = %s", msg)
	}
}
