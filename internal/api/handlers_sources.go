package api

import (
	"encoding/json"
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.ctrl.State())
}

func (h *Handlers) getSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"sources": h.ctrl.GetSources()})
}

func (h *Handlers) getSource(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	src, appErr := h.ctrl.GetSource(id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, src)
}

func (h *Handlers) setSource(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	var upd models.SourceUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.SetSource(r.Context(), id, upd)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
