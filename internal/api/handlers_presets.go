package api

import (
	"encoding/json"
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"presets": h.ctrl.GetPresets()})
}

func (h *Handlers) getPreset(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "pid")
	if err != nil {
		writeError(w, err)
		return
	}
	p, appErr := h.ctrl.GetPreset(id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handlers) createPreset(w http.ResponseWriter, r *http.Request) {
	var req models.PresetCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.CreatePreset(r.Context(), req)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusCreated, state)
}

func (h *Handlers) setPreset(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "pid")
	if err != nil {
		writeError(w, err)
		return
	}
	var upd models.PresetUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.SetPreset(r.Context(), id, upd)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) deletePreset(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "pid")
	if err != nil {
		writeError(w, err)
		return
	}
	state, appErr := h.ctrl.DeletePreset(r.Context(), id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) loadPreset(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "pid")
	if err != nil {
		writeError(w, err)
		return
	}
	state, appErr := h.ctrl.LoadPreset(r.Context(), id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
