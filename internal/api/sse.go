package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// sseEvents handles the SSE (Server-Sent Events) endpoint.
// Clients receive the current state immediately, then stream updates as they happen.
func (h *Handlers) sseEvents(w http.ResponseWriter, r *http.Request) {
	// Verify the client supports streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	id := uuid.New().String()
	ch := h.events.Subscribe(id)
	defer h.events.Unsubscribe(id)

	// Send current state immediately
	sendSSE(w, flusher, h.ctrl.State())

	for {
		select {
		case state, ok := <-ch:
			if !ok {
				return
			}
			sendSSE(w, flusher, state)
		case <-r.Context().Done():
			return
		}
	}
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
