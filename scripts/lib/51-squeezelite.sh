#!/usr/bin/env bash
# 51-squeezelite.sh — Build and install squeezelite (Logitech Media Server client)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Repo: CDrummond/squeezelite
# Build: make OPTS="$MARCH_FLAGS" ALSA=1 RESAMPLE=1 FFMPEG=1

set -euo pipefail

step "51 · squeezelite (LMS client)"

_name="squeezelite"
_repo="CDrummond/squeezelite"
_src_dir="$BUILD_DIR/squeezelite"
_bin="$INSTALL_PREFIX/bin/squeezelite"
_defaults_file="/etc/default/squeezelite"

# ── Version check ─────────────────────────────────────────────────────────────
_latest_tag="$(latest_github_tag "$_repo")"
log "Latest upstream tag: $_latest_tag"

_installed_ver="$(read_installed_version "$_name")"

if [[ -n "$_installed_ver" ]] && [[ "$_installed_ver" == "$_latest_tag" ]] && \
   [[ -x "$_bin" ]]; then
    skip "squeezelite $_installed_ver already installed"
    record_skip "squeezelite"
else
    log "Installing squeezelite ${_latest_tag} (installed: '${_installed_ver:-none}')"

    git_checkout_tag "https://github.com/${_repo}.git" "$_src_dir" "$_latest_tag"
    cd "$_src_dir"

    # squeezelite uses a flat Makefile — pass compile flags via OPTS
    make -j"$CORES" OPTS="$MARCH_FLAGS" ALSA=1 RESAMPLE=1 FFMPEG=1

    install -m 0755 squeezelite "$_bin"
    log "squeezelite installed to $_bin"

    write_installed_version "$_name" "$_latest_tag"
    record_done "squeezelite ${_latest_tag}"
fi

# ── Default runtime config (never overwrite) ──────────────────────────────────
step "squeezelite defaults"
if [[ -f "$_defaults_file" ]]; then
    skip "$_defaults_file (already exists)"
    record_skip "squeezelite defaults"
else
    cat > "$_defaults_file" <<'EOF'
# squeezelite defaults — edit to suit your setup
# Sound card: lb1 is AmpliPi loopback slot 1 (LMS source)
SL_SOUNDCARD="lb1"
# -C 5  close device after 5s of silence
# -W    report power state to LMS
SB_EXTRA_ARGS="-C 5 -W"
EOF
    log "wrote $_defaults_file"
    record_done "squeezelite defaults"
fi
