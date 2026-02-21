#!/usr/bin/env bash
# 70-amplipi.sh — Build and install the AmpliPi Go binary + systemd units
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# - Installs Go if missing
# - Builds cmd/amplipi/... and cmd/amplipi-update/... (if present)
# - Installs systemd unit files from scripts/configs/
# - Enables (but does NOT start) amplipi.service and amplipi-update.service

set -euo pipefail

step "70 · AmpliPi Go binary"

_this_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_repo_root="$(cd "$_this_dir/../.." && pwd)"
_configs_dir="$_this_dir/../configs"

# ── Go installation ───────────────────────────────────────────────────────────
step "Go toolchain"

_go_install_dir="/usr/local/go"
_go_bin="$_go_install_dir/bin/go"

_needs_go=1
if [[ -x "$_go_bin" ]]; then
    _go_ver="$("$_go_bin" version | awk '{print $3}')"  # e.g. go1.22.0
    log "Go already installed: $_go_ver"
    _needs_go=0
elif command -v go &>/dev/null; then
    _go_ver="$(go version | awk '{print $3}')"
    log "Go found in PATH: $_go_ver"
    _go_bin="$(command -v go)"
    _needs_go=0
fi

if (( _needs_go )); then
    log "Go not found — downloading latest stable release"

    # Determine arch for Go download
    case "$ARCH" in
        aarch64) _go_arch="arm64" ;;
        armv7l)  _go_arch="armv6l" ;;  # Go uses armv6l for all 32-bit ARM
        *)
            error "Unsupported arch for Go download: $ARCH"
            exit 1
            ;;
    esac

    # Get latest Go version from go.dev
    _go_latest="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1)"
    _go_tarball="${_go_latest}.linux-${_go_arch}.tar.gz"
    _go_url="https://go.dev/dl/${_go_tarball}"

    log "Downloading ${_go_tarball}"
    curl -fsSL -o "/tmp/${_go_tarball}" "$_go_url"

    # Remove old install and extract
    rm -rf "$_go_install_dir"
    tar -C /usr/local -xzf "/tmp/${_go_tarball}"
    rm -f "/tmp/${_go_tarball}"

    _go_bin="$_go_install_dir/bin/go"
    export PATH="$_go_install_dir/bin:$PATH"
    log "Go installed: $("$_go_bin" version)"
    record_done "Go toolchain"
else
    record_skip "Go toolchain (already installed)"
fi

# Ensure Go is in PATH for this session
export PATH="$(dirname "$_go_bin"):$PATH"

# ── Determine GOARCH / GOARM ──────────────────────────────────────────────────
case "$ARCH" in
    aarch64)
        export GOARCH="arm64"
        unset GOARM 2>/dev/null || true
        ;;
    armv7l)
        export GOARCH="arm"
        export GOARM="7"
        ;;
    *)
        warn "Unrecognised arch '$ARCH' — defaulting to GOARCH=arm64"
        export GOARCH="arm64"
        ;;
esac
export GOOS="linux"
export CGO_ENABLED="0"

log "Build target: GOOS=$GOOS GOARCH=$GOARCH${GOARM:+ GOARM=$GOARM}"

# ── Build amplipi ─────────────────────────────────────────────────────────────
step "Building amplipi binary"
_bin_dst="$INSTALL_PREFIX/bin/amplipi"

cd "$_repo_root"
"$_go_bin" build -ldflags="-s -w" -o "$_bin_dst" ./cmd/amplipi/...
log "amplipi installed to $_bin_dst"
record_done "amplipi binary"

# ── Build amplipi-update (if present) ────────────────────────────────────────
if [[ -d "$_repo_root/cmd/amplipi-update" ]] && [[ -n "$(find "$_repo_root/cmd/amplipi-update" -name '*.go' -print -quit)" ]]; then
    step "Building amplipi-update binary"
    _update_bin_dst="$INSTALL_PREFIX/bin/amplipi-update"
    "$_go_bin" build -ldflags="-s -w" -o "$_update_bin_dst" ./cmd/amplipi-update/...
    log "amplipi-update installed to $_update_bin_dst"
    record_done "amplipi-update binary"
else
    log "cmd/amplipi-update not found or empty — skipping (add later)"
fi

# ── Generate amplipi-display.service based on detected hardware ───────────────
# DISPLAY_MODE is set by check_pi() in common.sh:
#   "test-board"    → Pi 4/3/5 on standard 40-pin header (SPI0, --test-board flag)
#   "carrier-board" → Original AmpliPi CM3+ carrier board (SPI1, no --test-board)
step "Generating amplipi-display.service (display mode: $DISPLAY_MODE)"

_unit_dir="/etc/systemd/system"
_units_installed=0

if [[ "$DISPLAY_MODE" == "carrier-board" ]]; then
    _display_exec="/usr/local/bin/amplipi-display --addr :unix"
    _display_note="CM3+ carrier board — SPI1 (GPIO44), no --test-board flag"
else
    _display_exec="/usr/local/bin/amplipi-display --test-board --addr :unix"
    _display_note="Standard 40-pin header (Pi 4/3/5) — SPI0, --test-board flag"
fi

_display_svc_content="[Unit]
Description=AmpliPi Display Driver
# Detected at setup: $_display_note
# Re-run scripts/setup.sh to update this file if you change hardware.
After=network.target amplipi.service
BindsTo=amplipi.service

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi
ExecStart=$_display_exec
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=amplipi-display

[Install]
WantedBy=multi-user.target"

_display_dst="$_unit_dir/amplipi-display.service"
if [[ -f "$_display_dst" ]] && [[ "$(cat "$_display_dst")" == "$_display_svc_content" ]]; then
    skip "amplipi-display.service (unchanged, mode: $DISPLAY_MODE)"
    record_skip "amplipi-display.service"
else
    echo "$_display_svc_content" > "$_display_dst"
    log "wrote amplipi-display.service (mode: $DISPLAY_MODE)"
    (( _units_installed++ ))
    record_done "amplipi-display.service ($DISPLAY_MODE)"
fi

# ── Install remaining static systemd unit files ────────────────────────────────
step "systemd unit files (static)"

for _unit_src in "$_configs_dir"/*.service; do
    [[ -f "$_unit_src" ]] || continue
    _unit_name="$(basename "$_unit_src")"

    # Skip display service — generated dynamically above
    [[ "$_unit_name" == "amplipi-display.service" ]] && continue

    _unit_dst="$_unit_dir/$_unit_name"

    if [[ -f "$_unit_dst" ]] && diff -q "$_unit_src" "$_unit_dst" &>/dev/null; then
        skip "$_unit_name (unchanged)"
        record_skip "$_unit_name"
    else
        install -m 0644 "$_unit_src" "$_unit_dst"
        log "installed $_unit_name"
        (( _units_installed++ ))
        record_done "$_unit_name"
    fi
done

if (( _units_installed > 0 )) || [[ ! -f "$_unit_dir/amplipi.service" ]]; then
    systemctl daemon-reload
    log "systemctl daemon-reload done"
fi

# ── Enable services (do NOT start — user controls that) ──────────────────────
step "Enable systemd services (without starting)"
for _svc in amplipi.service amplipi-update.service amplipi-display.service; do
    if [[ ! -f "$_unit_dir/$_svc" ]]; then
        warn "$_svc not installed — skipping enable"
        continue
    fi
    if systemctl is-enabled --quiet "$_svc" 2>/dev/null; then
        skip "$_svc (already enabled)"
        record_skip "$_svc enable"
    else
        systemctl enable "$_svc"
        log "$_svc enabled (not started — run: sudo systemctl start $svc)"
        record_done "$_svc enable"
    fi
done
