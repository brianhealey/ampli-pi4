#!/usr/bin/env bash
# 53-gmrender.sh — Build and install gmrender-resurrect (DLNA/UPnP renderer)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Repo: hzeller/gmrender-resurrect
# Build: ./autogen.sh && ./configure --prefix=$INSTALL_PREFIX && make

set -euo pipefail

step "53 · gmrender-resurrect (DLNA/UPnP)"

_name="gmrender"
_repo="hzeller/gmrender-resurrect"
_src_dir="$BUILD_DIR/gmrender-resurrect"
_bin="$INSTALL_PREFIX/bin/gmediarender"

# ── Version check ─────────────────────────────────────────────────────────────
_latest_tag="$(latest_github_tag "$_repo")"
log "Latest upstream tag: $_latest_tag"

_installed_ver="$(read_installed_version "$_name")"

if [[ -n "$_installed_ver" ]] && [[ "$_installed_ver" == "$_latest_tag" ]] && \
   [[ -x "$_bin" ]]; then
    skip "gmrender $_installed_ver already installed"
    record_skip "gmrender"
else
    log "Installing gmrender ${_latest_tag} (installed: '${_installed_ver:-none}')"

    git_checkout_tag "https://github.com/${_repo}.git" "$_src_dir" "$_latest_tag"
    cd "$_src_dir"

    ./autogen.sh
    CFLAGS="$MARCH_FLAGS" ./configure --prefix="$INSTALL_PREFIX"
    make -j"$CORES"
    make install
    log "gmrender installed to $_bin"

    write_installed_version "$_name" "$_latest_tag"
    record_done "gmrender ${_latest_tag}"
fi
