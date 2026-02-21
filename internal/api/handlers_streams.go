package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/micro-nova/amplipi-go/internal/models"
)

func (h *Handlers) getStreams(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"streams": h.ctrl.GetStreams()})
}

func (h *Handlers) getStream(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	s, appErr := h.ctrl.GetStream(id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) createStream(w http.ResponseWriter, r *http.Request) {
	var req models.StreamCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.CreateStream(r.Context(), req)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusCreated, state)
}

func (h *Handlers) setStream(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	var upd models.StreamUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}
	state, appErr := h.ctrl.SetStream(r.Context(), id, upd)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) deleteStream(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	state, appErr := h.ctrl.DeleteStream(r.Context(), id)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handlers) execStreamCmd(w http.ResponseWriter, r *http.Request) {
	id, err := intParam(r, "sid")
	if err != nil {
		writeError(w, err)
		return
	}
	cmd := chi.URLParam(r, "cmd")
	if cmd == "" {
		writeError(w, models.ErrBadRequest("command is required"))
		return
	}
	state, appErr := h.ctrl.ExecStreamCommand(r.Context(), id, cmd)
	if appErr != nil {
		writeError(w, appErr)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
