package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/micro-nova/amplipi-go/internal/auth"
)

// NewRouter creates and returns the main HTTP router.
func NewRouter(ctrl Controller, authSvc *auth.Service, bus EventBus) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(corsMiddleware)
	r.Use(middleware.CleanPath)

	h := &Handlers{ctrl: ctrl, events: bus}

	// Auth routes (no auth required)
	r.Group(func(r chi.Router) {
		r.Get("/auth/login", h.loginPage)
		r.Post("/auth/login", h.loginPost)
	})

	// API routes (auth required)
	r.Group(func(r chi.Router) {
		r.Use(authSvc.Middleware)

		// System state
		r.Get("/api", h.getState)
		r.Get("/api/", h.getState)

		// Sources
		r.Get("/api/sources", h.getSources)
		r.Get("/api/sources/{sid}", h.getSource)
		r.Patch("/api/sources/{sid}", h.setSource)

		// Zones
		r.Get("/api/zones", h.getZones)
		r.Get("/api/zones/{zid}", h.getZone)
		r.Patch("/api/zones/{zid}", h.setZone)
		r.Patch("/api/zones", h.setZones)

		// Groups
		r.Get("/api/groups", h.getGroups)
		r.Get("/api/groups/{gid}", h.getGroup)
		r.Post("/api/group", h.createGroup)
		r.Patch("/api/groups/{gid}", h.setGroup)
		r.Delete("/api/groups/{gid}", h.deleteGroup)

		// Streams
		r.Get("/api/streams", h.getStreams)
		r.Get("/api/streams/{sid}", h.getStream)
		r.Post("/api/stream", h.createStream)
		r.Patch("/api/streams/{sid}", h.setStream)
		r.Delete("/api/streams/{sid}", h.deleteStream)
		r.Post("/api/streams/{sid}/{cmd}", h.execStreamCmd)

		// Presets
		r.Get("/api/presets", h.getPresets)
		r.Get("/api/presets/{pid}", h.getPreset)
		r.Post("/api/preset", h.createPreset)
		r.Patch("/api/presets/{pid}", h.setPreset)
		r.Delete("/api/presets/{pid}", h.deletePreset)
		r.Post("/api/presets/{pid}/load", h.loadPreset)

		// System
		r.Get("/api/info", h.getInfo)
		r.Post("/api/factory_reset", h.factoryReset)
		r.Post("/api/load", h.loadConfig)

		// SSE
		r.Get("/api/subscribe", h.sseEvents)
	})

	return r
}

// corsMiddleware adds permissive CORS headers for local network access.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, api-key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
