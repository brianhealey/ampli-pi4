// Command amplipi is the AmpliPi multi-zone audio system daemon.
// Run with --mock to use simulated hardware (no I2C device required).
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/micro-nova/amplipi-go/internal/api"
	"github.com/micro-nova/amplipi-go/internal/auth"
	"github.com/micro-nova/amplipi-go/internal/config"
	"github.com/micro-nova/amplipi-go/internal/controller"
	"github.com/micro-nova/amplipi-go/internal/events"
	"github.com/micro-nova/amplipi-go/internal/hardware"
	"github.com/micro-nova/amplipi-go/internal/maintenance"
	"github.com/micro-nova/amplipi-go/internal/models"
	"github.com/micro-nova/amplipi-go/internal/streams"
	"github.com/micro-nova/amplipi-go/internal/zeroconf"
)

//go:embed all:web/dist
var webFiles embed.FS

func main() {
	var (
		mock   = flag.Bool("mock", false, "use mock hardware driver (no I2C device required)")
		addr   = flag.String("addr", ":80", "HTTP listen address")
		cfgDir = flag.String("config-dir", "", "config directory (default: ~/.config/amplipi)")
		debug  = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	// Configure logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	// Resolve config directory
	if *cfgDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Error("cannot determine home directory", "err", err)
			os.Exit(1)
		}
		*cfgDir = filepath.Join(home, ".config", "amplipi")
	}
	if err := os.MkdirAll(*cfgDir, 0755); err != nil {
		slog.Error("cannot create config directory", "path", *cfgDir, "err", err)
		os.Exit(1)
	}

	// Graceful shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Hardware driver
	var hw hardware.Driver
	if *mock {
		slog.Info("using mock hardware driver")
		hw = hardware.NewMock()
	} else {
		slog.Info("using real I2C hardware driver")
		hw = hardware.NewI2C()
	}
	if err := hw.Init(ctx); err != nil {
		if !*mock {
			slog.Error("hardware initialization failed", "err", err)
			os.Exit(1)
		}
	}

	// Hardware profile detection
	profile, err := hardware.Detect(ctx, hw)
	if err != nil {
		slog.Warn("hardware detection failed, using mock defaults", "err", err)
		profile = hardware.MockProfile()
	}
	slog.Info("hardware profile",
		"units", len(profile.Units),
		"zones", profile.TotalZones,
		"sources", profile.TotalSources,
		"fan_mode", profile.FanMode,
		"display", profile.Display,
		"firmware", profile.FirmwareVersion,
	)
	slog.Info("stream capabilities", "available", profile.AvailableStreamTypes())

	// Config store
	store := config.NewJSONStore(*cfgDir)

	// Event bus
	bus := events.NewBus()

	// Stream manager
	// configDir for streams is ~/.config/amplipi/srcs/
	streamsConfigDir := filepath.Join(*cfgDir, "srcs")
	if err := os.MkdirAll(streamsConfigDir, 0755); err != nil {
		slog.Error("cannot create streams config directory", "path", streamsConfigDir, "err", err)
		os.Exit(1)
	}

	// Configure physical outputs availability from hardware profile
	streams.SetAvailablePhysicalOutputs(profile.AvailablePhysicalOutputs)

	// ctrlRef is used by the stream metadata callback to forward updates.
	// It is set after controller creation; callbacks only fire during stream
	// activity which happens after initialization.
	var ctrlRef *controller.Controller
	streamMgr := streams.NewManager(streamsConfigDir, func(id int, info models.StreamInfo) {
		if ctrlRef != nil {
			ctrlRef.UpdateStreamInfo(id, info)
		}
	})

	// Controller
	ctrl, err := controller.New(hw, profile, store, bus, streamMgr)
	if err != nil {
		slog.Error("controller initialization failed", "err", err)
		os.Exit(1)
	}
	ctrlRef = ctrl // safe: controller is initialized before any stream callbacks fire

	// Auth service
	authSvc, err := auth.NewService(*cfgDir)
	if err != nil {
		slog.Error("auth service initialization failed", "err", err)
		os.Exit(1)
	}
	defer authSvc.Close()

	// Maintenance goroutines (online check, release check, config backups)
	maint := maintenance.New(*cfgDir,
		func(online bool) {
			slog.Info("online status changed", "online", online)
		},
		func(release string) {
			slog.Info("new release available", "version", release)
		},
	)
	go maint.Start(ctx)

	// Zeroconf mDNS registration
	hostname, _ := os.Hostname()
	port := 80
	if parts := strings.SplitN(*addr, ":", 2); len(parts) == 2 && parts[1] != "" {
		if p, err := strconv.Atoi(parts[1]); err == nil {
			port = p
		}
	}
	zc := zeroconf.New(hostname, port)
	go func() {
		if err := zc.Start(ctx); err != nil {
			slog.Warn("zeroconf failed", "err", err)
		}
	}()

	// Background goroutines
	go hardware.RunPiTempSender(ctx, hw)

	// HTTP server
	router := api.NewRouter(ctrl, authSvc, bus)

	// Add web UI static file handler
	webFS, err := fs.Sub(webFiles, "web/dist")
	if err != nil {
		slog.Error("failed to load web files", "err", err)
		os.Exit(1)
	}
	router.(*chi.Mux).Handle("/*", http.FileServer(http.FS(webFS)))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 = no timeout (needed for SSE)
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("AmpliPi listening", "addr", *addr, "mock", *mock, "config", *cfgDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down...")

	// Shutdown stream manager
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	if err := streamMgr.Shutdown(shutCtx); err != nil {
		slog.Warn("stream manager shutdown error", "err", err)
	}

	// Flush pending config writes
	if err := store.Flush(); err != nil {
		slog.Warn("failed to flush config", "err", err)
	}

	// Graceful HTTP shutdown
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Warn("server shutdown error", "err", err)
	}

	slog.Info("shutdown complete")
}
