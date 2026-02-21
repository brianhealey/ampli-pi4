# amplipi-go

A Go replacement for the [AmpliPi](https://github.com/micro-nova/AmpliPi) multi-zone audio system daemon.

AmpliPi is a whole-home audio distribution system supporting up to 4 simultaneous sources and 36 amplified zones. This project re-implements the Python control daemon in Go for improved performance, reliability, and concurrent safety.

## Architecture

```
cmd/amplipi/          — Binary entry point
internal/
  models/             — Data structures (JSON-compatible with Python)
  hardware/           — I2C driver (real + mock) for STM32 preamp board
  config/             — Atomic JSON config persistence
  events/             — SSE event bus
  auth/               — Cookie/API-key authentication
  controller/         — State machine (sources, zones, groups, streams, presets)
  api/                — Chi HTTP router + REST handlers
web/                  — Frontend assets (embedded)
```

## Building

```bash
# Install dependencies
go mod tidy

# Build binary
make build

# Output: ./bin/amplipi
```

## Running

### Mock mode (no hardware required)

Run in mock mode for development and testing — no I2C device needed:

```bash
make run-mock
# or:
./bin/amplipi --mock --addr :8080
```

Then visit: `http://localhost:8080/api`

### Real hardware (Raspberry Pi + AmpliPi preamp)

```bash
sudo ./bin/amplipi --addr :80
```

Requires access to `/dev/i2c-1`. Run as root or add user to `i2c` group.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--mock` | false | Use mock hardware driver |
| `--addr` | `:80` | HTTP listen address |
| `--config-dir` | `~/.config/amplipi` | Config directory |
| `--debug` | false | Enable debug logging |

## API

The REST API is compatible with the Python AmpliPi API. All endpoints are under `/api/`:

- `GET /api` — Full system state
- `PATCH /api/sources/{sid}` — Update source
- `PATCH /api/zones/{zid}` — Update zone
- `PATCH /api/zones` — Bulk zone update
- `POST /api/group` / `PATCH /api/groups/{gid}` / `DELETE /api/groups/{gid}` — Group CRUD
- `POST /api/stream` / `PATCH /api/streams/{sid}` / `DELETE /api/streams/{sid}` — Stream CRUD
- `POST /api/streams/{sid}/{cmd}` — Stream command (play, pause, next, stop, etc.)
- `POST /api/preset` / `PATCH /api/presets/{pid}` / `DELETE /api/presets/{pid}` — Preset CRUD
- `POST /api/presets/{pid}/load` — Apply a preset
- `GET /api/subscribe` — SSE event stream
- `POST /api/factory_reset` — Reset to defaults
- `GET /api/info` — System info

## Development

```bash
make test    # Run tests
make lint    # Run linter (golangci-lint or go vet)
make tidy    # Tidy dependencies
make clean   # Remove binaries
```

## Config

Config is stored at `~/.config/amplipi/house.json` (JSON, compatible with Python format).
Config is written atomically (temp file + rename) with a 500ms debounce.

## Phase Status

- ✅ Phase 1: Models, hardware driver, config store, events, auth
- ✅ Phase 2: Controller state machine, REST API, SSE
- 🚧 Phase 3: Stream subprocess management (pianobar, shairport-sync, etc.)
- 🚧 Phase 4: Updates, display, full feature parity
