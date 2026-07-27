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

## Architecture

### Backend (Go 1.26.0)
- **cmd/amplipi/main.go**: Server bootstrap, embedding, graceful shutdown
- **internal/controller/**: Central state machine - ALL state mutations go through `apply()` method
- **internal/api/**: Chi HTTP router and REST handlers
- **internal/hardware/**: Hardware abstraction layer (I2C + mock implementations)
- **internal/models/**: Data structures (JSON-compatible with Python daemon)
- **internal/config/**: Atomic JSON persistence with 500ms debounce
- **internal/events/**: Pub-sub event bus for SSE delivery
- **internal/streams/**: Stream subprocess management (14 stream types)
- **internal/auth/**: Cookie/API-key authentication

### Frontend (Svelte 5 + SvelteKit)
- **web/src/lib/store.svelte.ts**: Singleton store using Svelte 5 Runes (`$state`, `$derived`)
- **web/src/lib/api.ts**: Promise-based fetch wrapper
- **web/src/routes/**: SvelteKit pages (home, zones, groups, streams)
- Uses Tailwind CSS 4.1, TypeScript strict mode
- Polls state every 2 seconds via `startPolling()`

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

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--mock` | false | Use mock hardware driver |
| `--addr` | `:80` | HTTP listen address |
| `--config-dir` | `~/.config/amplipi` | Config directory |
| `--debug` | false | Enable debug logging |

## Svelte MCP Tools (web/ directory)

When working on the frontend, use these MCP tools:
- **list-sections**: Discover Svelte 5/SvelteKit documentation
- **get-documentation**: Fetch relevant docs
- **svelte-autofixer**: Analyze Svelte code for issues before writing
