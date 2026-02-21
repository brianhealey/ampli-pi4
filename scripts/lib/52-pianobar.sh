#!/usr/bin/env bash
# 52-pianobar.sh — Build and install pianobar (Pandora client)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Repo: PromyLOPh/pianobar
# Build: cmake with $MARCH_FLAGS

set -euo pipefail

step "52 · pianobar (Pandora)"

_name="pianobar"
_repo="PromyLOPh/pianobar"
_src_dir="$BUILD_DIR/pianobar"
_bin="$INSTALL_PREFIX/bin/pianobar"

# ── Version check ─────────────────────────────────────────────────────────────
# pianobar uses date-style tags (e.g. 2023.05.21); try releases first
_latest_tag="$(latest_github_tag "$_repo")"
if [[ -z "$_latest_tag" ]]; then
    # No releases — use HEAD of default branch
    _latest_tag="HEAD"
fi
log "Latest upstream tag: $_latest_tag"

_installed_ver="$(read_installed_version "$_name")"

if [[ -n "$_installed_ver" ]] && [[ "$_installed_ver" == "$_latest_tag" ]] && \
   [[ -x "$_bin" ]]; then
    skip "pianobar $_installed_ver already installed"
    record_skip "pianobar"
else
    log "Installing pianobar ${_latest_tag} (installed: '${_installed_ver:-none}')"

    if [[ "$_latest_tag" == "HEAD" ]]; then
        # Clone or pull latest
        if [[ -d "$_src_dir/.git" ]]; then
            git -C "$_src_dir" pull --quiet
        else
            git clone --depth 1 "https://github.com/${_repo}.git" "$_src_dir" --quiet
        fi
    else
        git_checkout_tag "https://github.com/${_repo}.git" "$_src_dir" "$_latest_tag"
    fi

    cd "$_src_dir"

    # make build (pianobar uses Makefile, not CMake)
    make clean >/dev/null 2>&1 || true
    make -j"$CORES" \
        PREFIX="$INSTALL_PREFIX" \
        CFLAGS="$MARCH_FLAGS" \
        CC="gcc"

    make install PREFIX="$INSTALL_PREFIX"
    log "pianobar built and installed to $_bin"

    write_installed_version "$_name" "$_latest_tag"
    record_done "pianobar ${_latest_tag}"
fi

# ── Create user config directory (owned by invoking user, not root) ───────────
step "pianobar user config"
if [[ -z "${SUDO_USER:-}" ]]; then
    warn "SUDO_USER not set — skipping user config dir creation"
else
    _user_config_dir="$(getent passwd "$SUDO_USER" | cut -d: -f6)/.config/pianobar"
    if [[ -d "$_user_config_dir" ]]; then
        skip "pianobar config dir: $_user_config_dir"
        record_skip "pianobar config dir"
    else
        sudo -u "$SUDO_USER" mkdir -p "$_user_config_dir"
        log "created $_user_config_dir"
        record_done "pianobar config dir"
    fi

    # Hint file if config doesn't exist
    _config_file="$_user_config_dir/config"
    if [[ ! -f "$_config_file" ]]; then
        sudo -u "$SUDO_USER" tee "$_config_file" >/dev/null <<'EOF'
# pianobar configuration
# See: https://github.com/PromyLOPh/pianobar/wiki/Configuration
#
# user = your@pandora.email
# password = yourpassword
# autostart_station = your_station_id
EOF
        log "created placeholder pianobar config: $_config_file"
        record_done "pianobar placeholder config"
    else
        skip "pianobar config: $_config_file (already exists)"
    fi
fi
