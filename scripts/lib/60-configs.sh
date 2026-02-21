#!/usr/bin/env bash
# 60-configs.sh — Install AmpliPi default configuration files
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# NEVER overwrites files that already exist.
# All user-owned files are created as SUDO_USER, not root.

set -euo pipefail

step "60 · AmpliPi configuration files"

# ── Resolve the invoking user ─────────────────────────────────────────────────
if [[ -z "${SUDO_USER:-}" ]]; then
    warn "SUDO_USER not set — user-owned files will be owned by root. Re-run via sudo."
    _run_as_user="root"
    _user_home="$HOME"
else
    _run_as_user="$SUDO_USER"
    _user_home="$(getent passwd "$SUDO_USER" | cut -d: -f6)"
fi

# SCRIPT_DIR is relative to setup.sh location; resolve configs/ path
# When sourced, BASH_SOURCE[0] is THIS file, so we go up one level.
_this_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_configs_src="$_this_dir/../configs"

# ── Create $CONFIG_DIR (user-owned) ──────────────────────────────────────────
step "Config directory: $CONFIG_DIR"
if [[ -d "$CONFIG_DIR" ]]; then
    skip "$CONFIG_DIR (already exists)"
    record_skip "config dir"
else
    sudo -u "$_run_as_user" mkdir -p "$CONFIG_DIR"
    log "created $CONFIG_DIR"
    record_done "config dir"
fi

# ── house.json — default device configuration ─────────────────────────────────
step "house.json"
_dst_house="$CONFIG_DIR/house.json"
if [[ -f "$_dst_house" ]]; then
    skip "$_dst_house (already exists)"
    record_skip "house.json"
else
    if [[ -f "$_configs_src/house.json" ]]; then
        sudo -u "$_run_as_user" cp "$_configs_src/house.json" "$_dst_house"
        log "installed house.json → $_dst_house"
        record_done "house.json"
    else
        warn "Source not found: $_configs_src/house.json — skipping"
    fi
fi

# Ensure ownership is correct regardless
if [[ -f "$_dst_house" ]] && [[ "$_run_as_user" != "root" ]]; then
    chown "${_run_as_user}:${_run_as_user}" "$_dst_house"
fi

# ── Log directory ─────────────────────────────────────────────────────────────
step "Log directory /var/log/amplipi"
if [[ -d "/var/log/amplipi" ]]; then
    skip "/var/log/amplipi (already exists)"
    record_skip "log dir"
else
    mkdir -p "/var/log/amplipi"
    if [[ "$_run_as_user" != "root" ]]; then
        chown "${_run_as_user}:${_run_as_user}" "/var/log/amplipi"
    fi
    log "created /var/log/amplipi"
    record_done "log dir"
fi

# ── Runtime directory ─────────────────────────────────────────────────────────
step "Runtime directory /var/run/amplipi"
if [[ -d "/var/run/amplipi" ]]; then
    skip "/var/run/amplipi (already exists)"
    record_skip "run dir"
else
    mkdir -p "/var/run/amplipi"
    if [[ "$_run_as_user" != "root" ]]; then
        chown "${_run_as_user}:${_run_as_user}" "/var/run/amplipi"
    fi
    log "created /var/run/amplipi"
    record_done "run dir"
fi
