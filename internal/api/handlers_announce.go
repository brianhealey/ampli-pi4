package api

import (
	"encoding/json"
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/models"
)

// announce handles POST /api/announce
// Makes a PA-style announcement on one or more zones.
//
// The announcement:
// - Saves the current state
// - Creates a temporary file player stream with the media URL
// - Connects the announcement to specified zones (or all enabled zones if none specified)
// - Waits for the announcement to finish playing (blocking)
// - Restores the previous state
//
// This endpoint blocks until the announcement completes or times out.
func (h *Handlers) announce(w http.ResponseWriter, r *http.Request) {
	var req models.AnnounceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, models.ErrBadRequest("invalid JSON: "+err.Error()))
		return
	}

	state, appErr := h.ctrl.Announce(r.Context(), req)
	if appErr != nil {
		writeError(w, appErr)
		return
	}

	writeJSON(w, http.StatusOK, state)
}
