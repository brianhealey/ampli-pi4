// Package zeroconf registers the AmpliPi web UI as an mDNS/DNS-SD service
// so it is discoverable on the LAN as amplipi.local.
package zeroconf

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/grandcat/zeroconf"
)

// Service manages mDNS service registration.
type Service struct {
	name   string // instance name / hostname, e.g. "amplipi"
	port   int
	server *zeroconf.Server
}

// New creates a new zeroconf Service that will advertise on the given port.
// name should be the hostname (e.g. "amplipi").
func New(name string, port int) *Service {
	return &Service{
		name: name,
		port: port,
	}
}

// Start registers the mDNS service and blocks until ctx is cancelled, at which
// point it shuts down the server cleanly.
func (s *Service) Start(ctx context.Context) error {
	txt := []string{"version=0.5.0-go", "model=AmpliPi"}

	server, err := zeroconf.Register(
		s.name,     // instance name
		"_http._tcp", // service type
		"local.",     // domain
		s.port,       // port
		txt,          // TXT records
		nil,          // ifaces — nil means all interfaces
	)
	if err != nil {
		return fmt.Errorf("zeroconf register: %w", err)
	}
	s.server = server
	slog.Info("zeroconf: registered mDNS service",
		"name", s.name,
		"port", s.port,
		"txt", txt,
	)

	<-ctx.Done()

	server.Shutdown()
	slog.Info("zeroconf: mDNS service unregistered")
	return nil
}

// UpdateTXT updates the TXT records for the registered service.
// Note: grandcat/zeroconf v1.0.0 does not expose a SetText method; to update
// TXT records the server must be restarted. This is a best-effort operation.
func (s *Service) UpdateTXT(records []string) error {
	if s.server == nil {
		return fmt.Errorf("zeroconf: server not started")
	}
	// The grandcat/zeroconf library does not provide a live TXT update API.
	// Log the intended update; callers should restart the service to apply changes.
	slog.Info("zeroconf: TXT update requested (requires service restart to apply)", "records", records)
	return nil
}
