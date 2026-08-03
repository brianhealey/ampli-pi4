package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/micro-nova/amplipi-go/internal/auth"
	"github.com/micro-nova/amplipi-go/internal/logs"
	"github.com/micro-nova/amplipi-go/internal/system"
)

// Options collects the dependencies of the HTTP router. Only Controller, Auth
// and Bus are required; the diagnostics fields disable their endpoints when nil.
type Options struct {
	Controller Controller
	Auth       *auth.Service
	Bus        EventBus
	ConfigDir  string            // config directory for backup operations
	System     *system.Collector // device health telemetry
	Logs       *logs.Service     // log reader
}

// NewRouter creates and returns the main HTTP router.
func NewRouter(ctrl Controller, authSvc *auth.Service, bus EventBus) http.Handler {
	return NewRouterWithConfig(ctrl, authSvc, bus, "")
}

// NewRouterWithConfig creates a router with explicit config directory for backup operations.
func NewRouterWithConfig(ctrl Controller, authSvc *auth.Service, bus EventBus, configDir string) http.Handler {
	return NewRouterWithOptions(Options{
		Controller: ctrl,
		Auth:       authSvc,
		Bus:        bus,
		ConfigDir:  configDir,
	})
}

// NewRouterWithOptions creates a router from an explicit dependency set.
func NewRouterWithOptions(opts Options) http.Handler {
	ctrl, authSvc, bus, configDir := opts.Controller, opts.Auth, opts.Bus, opts.ConfigDir
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(securityHeadersMiddleware)
	r.Use(corsMiddleware)
	r.Use(middleware.CleanPath)
	r.Use(maxBytesMiddleware(1 << 20)) // 1MB default limit for JSON bodies

	h := &Handlers{
		ctrl:      ctrl,
		events:    bus,
		auth:      authSvc,
		configDir: configDir,
		system:    opts.System,
		logs:      opts.Logs,
	}

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

		// Announcements
		r.Post("/api/announce", h.announce)

		// System
		r.Get("/api/info", h.getInfo)
		r.Post("/api/factory_reset", h.factoryReset)
		r.Post("/api/load", h.loadConfig)

		// Diagnostics
		r.Get("/api/system/stats", h.getSystemStats)
		r.Get("/api/system/hardware", h.getHardwareStatus)
		r.Get("/api/system/logs", h.getLogs)
		r.Get("/api/system/logs/sources", h.getLogSources)

		// Hardware tests
		r.Post("/api/test/preamp", h.testPreamp)
		r.Post("/api/test/fans", h.testFans)

		// Firmware (stub)
		r.Post("/api/firmware/flash", h.flashFirmware)

		// Backup/restore
		r.Post("/api/backup", h.createBackup)
		r.Get("/api/backup", h.listBackups)
		r.Post("/api/restore", h.restoreBackup)

		// SSE
		r.Get("/api/subscribe", h.sseEvents)
	})

	return r
}

// corsMiddleware adds CORS headers for local network access.
// It validates the Origin header against a list of allowed patterns
// to prevent cross-site request forgery from malicious websites.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Allow requests with no Origin (same-origin requests, curl, etc.)
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if origin is allowed
		if isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Key, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight for 24 hours

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else {
			// For disallowed origins, don't set any CORS headers
			// The browser will block the response
			if r.Method == http.MethodOptions {
				// Reject preflight requests from disallowed origins
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isAllowedOrigin checks if an origin is allowed for CORS.
// Allowed origins:
// - localhost with any port (development)
// - Private network IPs (192.168.x.x, 10.x.x.x, 172.16-31.x.x)
// - .local domains (mDNS/Bonjour names like amplipi.local)
func isAllowedOrigin(origin string) bool {
	// Remove protocol prefix for comparison
	host := origin
	if strings.HasPrefix(host, "http://") {
		host = strings.TrimPrefix(host, "http://")
	} else if strings.HasPrefix(host, "https://") {
		host = strings.TrimPrefix(host, "https://")
	}

	// Remove port suffix for hostname checks
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		// Be careful with IPv6 addresses
		if !strings.Contains(host, "[") {
			host = host[:colonIdx]
		}
	}

	// Allow localhost
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	// Allow .local domains (mDNS/Bonjour)
	if strings.HasSuffix(host, ".local") {
		return true
	}

	// Allow private network ranges
	// 10.0.0.0/8
	if strings.HasPrefix(host, "10.") {
		return true
	}

	// 192.168.0.0/16
	if strings.HasPrefix(host, "192.168.") {
		return true
	}

	// 172.16.0.0/12 (172.16.x.x - 172.31.x.x)
	if strings.HasPrefix(host, "172.") {
		parts := strings.Split(host, ".")
		if len(parts) >= 2 {
			// Check if second octet is 16-31
			switch parts[1] {
			case "16", "17", "18", "19", "20", "21", "22", "23",
				"24", "25", "26", "27", "28", "29", "30", "31":
				return true
			}
		}
	}

	return false
}

// securityHeadersMiddleware adds security headers to all responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")
		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Disable caching for API responses (but allow for static assets)
		if strings.HasPrefix(r.URL.Path, "/api") {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}

// maxBytesMiddleware limits request body size to prevent memory exhaustion.
func maxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip limit for multipart form (handled separately in restoreBackup)
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				next.ServeHTTP(w, r)
				return
			}
			// Limit body size for JSON requests
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
