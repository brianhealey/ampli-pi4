package api

import (
	"encoding/json"
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getGroups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"groups": h.ctrl.GetGroups()})
}

func (h *Handlers) getGroup(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "gid")
	if err != nil {
		writeError(w, err)
		return
	}
	g, appErr := h.ctrl.GetGroup(id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handlers) createGroup(w http.ResponseWriter, r *http.Request) {
	var req models.GroupUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.CreateGroup(r.Context(), req)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusCreated, state)
}

func (h *Handlers) setGroup(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "gid")
	if err != nil {
		writeError(w, err)
		return
	}
	var upd models.GroupUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.SetGroup(r.Context(), id, upd)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "gid")
	if err != nil {
		writeError(w, err)
		return
	}
	state, appErr := h.ctrl.DeleteGroup(r.Context(), id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
