# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AmpliPi-Go is a Go-based replacement daemon for the Python AmpliPi whole-home audio distribution system. It controls up to 4 audio sources and 36 amplified zones via I2C hardware abstraction, with a Svelte 5 web UI.

## Build & Development Commands

```bash
# Build
make build              # Local binary with embedded web UI
make build-pi          # ARM64 cross-compile for Raspberry Pi
make web-build         # Rebuild web UI only

# Run
make run-mock          # Local mock mode on :8080 (no hardware)
make run               # Local real hardware mode on :80
./bin/amplipi --mock --addr :8080  # Direct invocation

# Test & Lint
make test              # All tests with race detector
make test-short        # Tests without race detector
make lint              # golangci-lint or go vet
make tidy              # go mod tidy

# Deploy to Pi
make deploy            # Build and deploy binary
make deploy-run        # Deploy and restart service
make deploy-logs       # Tail systemd journal
```

## Architecture Notes

Layout and dependencies are discoverable from the tree and manifests. The
non-obvious parts:

- `internal/config/` persists atomically with a **500ms debounce** — writes are
  coalesced, so a config change is not on disk the instant `apply()` returns.
- `internal/streams/` supervises **14 stream types** as subprocesses.
- The web UI polls `GET /api` every **2 seconds** (`startPolling()`); there is
  also an SSE endpoint at `/api/subscribe`.

## Key Patterns

### State Management
All state changes must go through `Controller.apply()`:
```go
func (c *Controller) apply(fn func(*models.State) error) (models.State, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    // mutate, persist, publish event
}
```

### JSON Field Naming
Use snake_case for JSON tags (Python compatibility):
```go
type Zone struct {
    SourceID int `json:"source_id"`
}
```

### Error Handling
Use structured `AppError` with HTTP status codes:
```go
ErrNotFound("zone not found")    // 404
ErrBadRequest("invalid volume")  // 400
ErrInternal("hardware failure")  // 500
```

### Hardware Abstraction
All hardware operations accept context for cancellation:
```go
func (d *I2C) SetZoneVol(ctx context.Context, unit, zone int, vol int) error
```

### Concurrency
- `Controller` holds `sync.RWMutex` for state protection
- `streams.Manager` holds `sync.Mutex` for lifecycle management
- Event bus uses non-blocking publishes (drops events for slow subscribers)

## Configuration

- **Main config**: `~/.config/amplipi/house.json`
- **User auth**: `~/.config/amplipi/users.json`
- **Stream metadata**: `~/.config/amplipi/srcs/`
- **systemd service**: `/etc/systemd/system/amplipi.service`

Run `./bin/amplipi --help` for the current CLI flags.

Frontend guidance lives in `web/CLAUDE.md`, which loads automatically when
working under `web/`.
