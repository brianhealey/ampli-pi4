// Command amplipi-display is the AmpliPi front-panel display driver.
// It polls the AmpliPi API and renders system status on the TFT or eInk display.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Config holds the display driver configuration.
type Config struct {
	APIURL     string // URL of the AmpliPi API
	UpdateRate int    // Update rate in seconds
	LogLevel   string // Log level (debug, info, warn, error)
}

// Status represents system status for display.
type Status struct {
	Hostname     string
	IP           string
	Password     string
	DiskUsedGB   float64
	DiskTotalGB  float64
	DiskPercent  float64
	Sources      []SourceInfo
	Zones        []ZoneInfo
	Expanders    int
}

// SourceInfo holds source display information.
type SourceInfo struct {
	ID      int
	Name    string
	Playing bool
}

// ZoneInfo holds zone display information.
type ZoneInfo struct {
	ID     int
	Name   string
	Mute   bool
	Volume int // -79 to 0 dB
}

func main() {
	// Parse flags
	var (
		addr       = flag.String("addr", "localhost", "AmpliPi API address")
		updateRate = flag.Int("update-rate", 1, "Display update rate in seconds")
		logLevel   = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	// Configure logging
	level := slog.LevelInfo
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	// Normalize address - ":unix" means "localhost" for backward compatibility
	apiHost := *addr
	if apiHost == ":unix" || apiHost == "unix" {
		apiHost = "localhost"
	}

	cfg := Config{
		APIURL:     fmt.Sprintf("http://%s/api", apiHost),
		UpdateRate: *updateRate,
		LogLevel:   *logLevel,
	}

	slog.Info("amplipi-display starting", "api", cfg.APIURL, "rate", cfg.UpdateRate)

	// Check for TFT display hardware
	// TODO: Implement actual hardware detection via SPI
	displayType := detectDisplay()
	if displayType == "none" {
		slog.Warn("no display hardware detected, running in log-only mode")
	} else {
		slog.Info("display hardware detected", "type", displayType)
	}

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Run display update loop
	if err := run(ctx, cfg, displayType); err != nil {
		slog.Error("display driver failed", "err", err)
		os.Exit(1)
	}

	slog.Info("amplipi-display stopped")
}

// detectDisplay checks for TFT or eInk display hardware.
// Returns "tft", "eink", or "none".
func detectDisplay() string {
	// TODO: Implement actual SPI hardware detection
	// For now, return "tft" since user has TFT display
	// In a full implementation, this would:
	// 1. Try to open SPI device (/dev/spidev1.0 for CM4S)
	// 2. Send ILI9341 device ID read command
	// 3. If successful, return "tft"
	// 4. Otherwise try eInk detection
	// 5. If both fail, return "none"
	return "tft"
}

// run executes the main display update loop.
func run(ctx context.Context, cfg Config, displayType string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(time.Duration(cfg.UpdateRate) * time.Second)
	defer ticker.Stop()

	consecutiveErrors := 0
	const maxConsecutiveErrors = 10

	slog.Info("display update loop started")

	// Initial update
	if err := updateDisplay(ctx, client, cfg, displayType); err != nil {
		slog.Warn("initial display update failed", "err", err)
		consecutiveErrors++
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := updateDisplay(ctx, client, cfg, displayType); err != nil {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					return fmt.Errorf("too many consecutive errors (%d): %w", consecutiveErrors, err)
				}
				slog.Warn("display update failed", "err", err, "consecutive_errors", consecutiveErrors)
			} else {
				consecutiveErrors = 0
			}
		}
	}
}

// updateDisplay fetches status from API and updates the display.
func updateDisplay(ctx context.Context, client *http.Client, cfg Config, displayType string) error {
	// Fetch status from API
	status, err := fetchStatus(ctx, client, cfg.APIURL)
	if err != nil {
		return fmt.Errorf("fetch status: %w", err)
	}

	// Render to display
	if err := render(status, displayType); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}

// fetchStatus retrieves system status from the AmpliPi API.
func fetchStatus(ctx context.Context, client *http.Client, apiURL string) (*Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse API response
	var apiResp struct {
		Sources []struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Input string `json:"input"`
		} `json:"sources"`
		Zones []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Mute   bool   `json:"mute"`
			Vol    int    `json:"vol"`
			Source int    `json:"source_id"`
		} `json:"zones"`
		Streams []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
			Info struct {
				Name  string `json:"name"`
				State string `json:"state"`
			} `json:"info"`
		} `json:"streams"`
		Info struct {
			Version string `json:"version"`
			Offline bool   `json:"offline"`
			Units   int    `json:"units"`
		} `json:"info"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Get hostname and IP
	hostname, _ := os.Hostname()
	ip := getLocalIP()

	// Get disk usage
	diskUsedGB, diskTotalGB, diskPercent := getDiskUsage()

	// Build source info
	sources := make([]SourceInfo, len(apiResp.Sources))
	for i, src := range apiResp.Sources {
		// Determine if source is playing by checking stream state
		playing := false
		// Extract stream ID from input (e.g., "stream=0")
		if len(src.Input) > 7 && src.Input[:7] == "stream=" {
			streamID := int(src.Input[7] - '0')
			if streamID >= 0 && streamID < len(apiResp.Streams) {
				playing = apiResp.Streams[streamID].Info.State == "playing"
			}
		}

		sources[i] = SourceInfo{
			ID:      src.ID,
			Name:    src.Name,
			Playing: playing,
		}
	}

	// Build zone info
	zones := make([]ZoneInfo, len(apiResp.Zones))
	for i, z := range apiResp.Zones {
		zones[i] = ZoneInfo{
			ID:     z.ID,
			Name:   z.Name,
			Mute:   z.Mute,
			Volume: z.Vol,
		}
	}

	// Calculate expander count (units - 1, since unit 0 is master)
	expanders := 0
	if apiResp.Info.Units > 1 {
		expanders = apiResp.Info.Units - 1
	}

	return &Status{
		Hostname:    hostname,
		IP:          ip,
		Password:    getPassword(),
		DiskUsedGB:  diskUsedGB,
		DiskTotalGB: diskTotalGB,
		DiskPercent: diskPercent,
		Sources:     sources,
		Zones:       zones,
		Expanders:   expanders,
	}, nil
}

// getDiskUsage returns disk usage statistics.
func getDiskUsage() (usedGB, totalGB, percent float64) {
	// TODO: Implement actual disk usage check via syscall.Statfs
	// For now, return placeholder values
	return 7.2, 29.0, 24.8
}

// getPassword reads the default password from config file.
func getPassword() string {
	// TODO: Read from ~/.config/amplipi/default_password.txt
	// For now, return default
	return "raspberry"
}

// render displays the status on the appropriate hardware.
func render(status *Status, displayType string) error {
	switch displayType {
	case "tft":
		return renderTFT(status)
	case "eink":
		return renderEInk(status)
	case "none":
		return renderLog(status)
	default:
		return fmt.Errorf("unknown display type: %s", displayType)
	}
}

// Global TFT instance
var tftDisplay *TFT

// renderTFT renders status to the TFT display.
func renderTFT(status *Status) error {
	// Initialize TFT on first call
	if tftDisplay == nil {
		var err error
		tftDisplay, err = NewTFT()
		if err != nil {
			// If TFT init fails, log and continue (fall back to log-only mode)
			slog.Warn("TFT init failed, falling back to log-only mode", "err", err)
			return renderLog(status)
		}
	}

	// Render status to TFT
	if err := tftDisplay.RenderStatus(status); err != nil {
		return fmt.Errorf("render to TFT: %w", err)
	}

	slog.Debug("TFT display updated successfully")
	return nil
}

// renderEInk renders status to the eInk display.
func renderEInk(status *Status) error {
	// TODO: Implement eInk rendering
	slog.Debug("eInk display update", "hostname", status.Hostname, "ip", status.IP)
	return nil
}

// renderLog logs the status (for when no hardware is present).
func renderLog(status *Status) error {
	playing := 0
	muted := 0
	for _, z := range status.Zones {
		if z.Mute {
			muted++
		} else {
			playing++
		}
	}

	slog.Info("display status",
		"hostname", status.Hostname,
		"ip", status.IP,
		"password", status.Password,
		"disk", fmt.Sprintf("%.1f/%.1f GB (%.1f%%)", status.DiskUsedGB, status.DiskTotalGB, status.DiskPercent),
		"zones", fmt.Sprintf("▶%d ⏸%d (total: %d)", playing, muted, len(status.Zones)),
		"expanders", status.Expanders,
	)
	return nil
}

// getLocalIP returns the local IP address (best effort).
func getLocalIP() string {
	// Simple approach: try to dial out and see what local address is used
	// This doesn't actually connect, just resolves the routing
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "unknown"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
