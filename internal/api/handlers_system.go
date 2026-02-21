package api

import (
	"encoding/json"
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.ctrl.GetInfo())
}

func (h *Handlers) factoryReset(w http.ResponseWriter, r *http.Request) {
	state, appErr := h.ctrl.FactoryReset(r.Context())
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) loadConfig(w http.ResponseWriter, r *http.Request) {
	var incoming models.State
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.LoadConfig(r.Context(), incoming)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// loginPage renders a simple login HTML page.
func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if next == "" {
		next = "/api"
	}
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>AmpliPi Login</title></head>
<body>
<h2>AmpliPi Login</h2>
<form method="POST" action="/auth/login">
  <input type="hidden" name="next" value="` + next + `">
  <label>Password: <input type="password" name="password"></label>
  <button type="submit">Login</button>
</form>
</body>
</html>`))
}

// loginPost handles login form submission.
// TODO: implement proper credential verification with argon2.
func (h *Handlers) loginPost(w http.ResponseWriter, r *http.Request) {
	// For now, redirect to requested URL (auth service handles actual verification).
	next := r.FormValue("next")
	if next == "" {
		next = "/api"
	}
	http.Redirect(w, r, next, http.StatusFound)
}
