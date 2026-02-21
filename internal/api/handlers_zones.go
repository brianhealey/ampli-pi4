package api

import (
	"encoding/json"
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getZones(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"zones": h.ctrl.GetZones()})
}

func (h *Handlers) getZone(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "zid")
	if err != nil {
		writeError(w, err)
		return
	}
	z, appErr := h.ctrl.GetZone(id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, z)
}

func (h *Handlers) setZone(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "zid")
	if err != nil {
		writeError(w, err)
		return
	}
	var upd models.ZoneUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.SetZone(r.Context(), id, upd)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) setZones(w http.ResponseWriter, r *http.Request) {
	var req models.MultiZoneUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.SetZones(r.Context(), req)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
