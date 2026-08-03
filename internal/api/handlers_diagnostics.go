package api

import (
	"net/http"

	"github.com/micro-nova/amplipi-go/internal/logs"
	"github.com/micro-nova/amplipi-go/internal/models"
)

// getSystemStats returns a live snapshot of device health: CPU, memory, disk,
// network, thermal and daemon process statistics.
func (h *Handlers) getSystemStats(w http.ResponseWriter, r *http.Request) {
	if h.system == nil {
		writeError(w, models.ErrInternal("system stats collector not configured"))
		return
	}
	writeJSON(w, http.StatusOK, h.system.Collect(r.Context()))
}

// getHardwareStatus returns the detected controller topology with per-unit
// firmware versions and live telemetry.
func (h *Handlers) getHardwareStatus(w http.ResponseWriter, r *http.Request) {
	provider, ok := h.ctrl.(HardwareStatusProvider)
	if !ok {
		writeError(w, models.ErrInternal("controller does not expose hardware status"))
		return
	}
	writeJSON(w, http.StatusOK, provider.HardwareStatus(r.Context()))
}

// getLogSources lists the log streams available for reading.
func (h *Handlers) getLogSources(w http.ResponseWriter, r *http.Request) {
	if h.logs == nil {
		writeError(w, models.ErrInternal("log service not configured"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sources": h.logs.Sources(),
	})
}

// getLogs reads recent entries from one log source.
//
// Query parameters:
//
//	source  one of the ids from GET /api/system/logs/sources (default "app")
//	lines   maximum entries to return (default 200, max 5000)
//	since   relative window such as "30m", "2h", "7d"
//	level   minimum severity: debug | info | warn | error
//	filter  case-insensitive substring match on the message or unit
func (h *Handlers) getLogs(w http.ResponseWriter, r *http.Request) {
	if h.logs == nil {
		writeError(w, models.ErrInternal("log service not configured"))
		return
	}

	params := r.URL.Query()
	source := params.Get("source")
	if source == "" {
		source = logs.AppSource
	}

	query, err := logs.NewQuery(
		source,
		params.Get("lines"),
		params.Get("since"),
		params.Get("filter"),
		params.Get("level"),
	)
	if err != nil {
		writeError(w, models.ErrBadRequest(err.Error()))
		return
	}

	result, err := h.logs.Read(r.Context(), query)
	if err != nil {
		writeError(w, models.ErrBadRequest(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, result)
}
