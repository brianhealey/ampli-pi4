#!/usr/bin/env bash
# 55-bluealsa.sh — Build and install bluez-alsa (Bluetooth audio)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Repo: Arkq/bluez-alsa
# Build: autoreconf --install && ./configure --enable-aac --enable-faststream && make

set -euo pipefail

step "55 · bluez-alsa (Bluetooth audio)"

_name="bluealsa"
_repo="Arkq/bluez-alsa"
_src_dir="$BUILD_DIR/bluez-alsa"

# ── Version check ─────────────────────────────────────────────────────────────
_latest_tag="$(latest_github_tag "$_repo")"
log "Latest upstream tag: $_latest_tag"

_installed_ver="$(read_installed_version "$_name")"

if [[ -n "$_installed_ver" ]] && [[ "$_installed_ver" == "$_latest_tag" ]] && \
   command -v bluealsa &>/dev/null; then
    skip "bluealsa $_installed_ver already installed"
    record_skip "bluealsa"
else
    log "Installing bluealsa ${_latest_tag} (installed: '${_installed_ver:-none}')"

    git_checkout_tag "https://github.com/${_repo}.git" "$_src_dir" "$_latest_tag"
    cd "$_src_dir"

    autoreconf --install --force >/dev/null 2>&1
    CFLAGS="$MARCH_FLAGS" ./configure \
        --prefix="$INSTALL_PREFIX" \
        --enable-faststream \
        --with-libbluetooth

    make -j"$CORES"
    make install
    log "bluealsa built and installed"

    write_installed_version "$_name" "$_latest_tag"
    record_done "bluealsa ${_latest_tag}"
fi

# ── Enable bluealsa systemd service ──────────────────────────────────────────
step "bluealsa systemd service"
if systemctl is-enabled --quiet bluealsa 2>/dev/null; then
    skip "bluealsa.service (already enabled)"
    record_skip "bluealsa.service"
else
    systemctl daemon-reload
    systemctl enable bluealsa 2>/dev/null || \
        warn "bluealsa.service unit not found — it may be installed as bluez-alsa; check after reboot"
    log "bluealsa.service enabled"
    record_done "bluealsa.service"
fi

# ── Add pi user to bluetooth group ───────────────────────────────────────────
step "bluetooth group membership"
if id -nG pi 2>/dev/null | grep -qw bluetooth; then
    skip "pi is already in bluetooth group"
    record_skip "pi bluetooth group"
else
    if id pi &>/dev/null; then
        usermod -aG bluetooth pi
        log "added pi to bluetooth group"
        record_done "pi bluetooth group"
    else
        warn "User 'pi' not found — skipping bluetooth group"
    fi
fi
