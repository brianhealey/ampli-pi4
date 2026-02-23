#!/usr/bin/env bash
# setup.sh — AmpliPi Raspberry Pi 4 device setup
#
# Usage:
#   sudo scripts/setup.sh [--skip-build]
#
#   --skip-build   Skip all binary build steps (50-55, 70).
#                  Useful for re-runs after an initial build.
#
# This script is idempotent: safe to run multiple times.
# It sources numbered lib scripts in order and prints a summary at the end.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="$SCRIPT_DIR/lib"

# ── Parse arguments ──────────────────────────────────────────────────────────
SKIP_BUILD=0
for arg in "$@"; do
    case "$arg" in
        --skip-build) SKIP_BUILD=1 ;;
        --help|-h)
            echo "Usage: sudo $0 [--skip-build]"
            echo ""
            echo "Options:"
            echo "  --skip-build   Skip binary build steps (50-55, 70)"
            exit 0
            ;;
        *)
            echo "Unknown argument: $arg" >&2
            exit 1
            ;;
    esac
done

# ── Bootstrap common.sh ──────────────────────────────────────────────────────
# shellcheck source=lib/common.sh
source "$LIB_DIR/common.sh"

require_root

echo ""
echo "╔══════════════════════════════════════════════════════╗"
echo "║     AmpliPi — Raspberry Pi 4 Device Setup           ║"
echo "╚══════════════════════════════════════════════════════╝"
echo ""

check_pi

if [[ "$SKIP_BUILD" -eq 1 ]]; then
    warn "--skip-build: binary build steps (50-55, 70) will be skipped"
fi

mkdir -p "$BUILD_DIR"

# ── Helper: source a lib script ──────────────────────────────────────────────
run_lib() {
    local script="$LIB_DIR/$1"
    if [[ ! -f "$script" ]]; then
        error "Missing lib script: $script"
        exit 1
    fi
    # shellcheck disable=SC1090
    source "$script"
}

# ── Run scripts in order ─────────────────────────────────────────────────────
run_lib "05-docker.sh"
run_lib "10-system.sh"
run_lib "20-hardware.sh"
run_lib "30-alsa.sh"
run_lib "40-deps.sh"

if [[ "$SKIP_BUILD" -eq 0 ]]; then
    # NOTE: Streaming services now run in Docker containers
    # Commenting out to avoid conflicts during Docker migration
    # run_lib "50-shairport-sync.sh"
    # run_lib "51-squeezelite.sh"
    # run_lib "52-pianobar.sh"
    # run_lib "53-gmrender.sh"
    # run_lib "54-go-librespot.sh"
    # run_lib "55-bluealsa.sh"
    log "Streaming services will run in Docker containers - skipping bare-metal builds"
else
    warn "Skipping build scripts 50-55 (--skip-build)"
fi

run_lib "60-configs.sh"

if [[ "$SKIP_BUILD" -eq 0 ]]; then
    # NOTE: AmpliPi binary now runs in Docker container
    # Commenting out to avoid conflicts during Docker migration
    # run_lib "70-amplipi.sh"
    log "AmpliPi binary will run in Docker container - skipping bare-metal build"
else
    warn "Skipping build script 70 (--skip-build)"
fi

# ── Final summary ─────────────────────────────────────────────────────────────
print_summary
