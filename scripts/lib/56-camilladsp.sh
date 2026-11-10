#!/usr/bin/env bash
# 56-camilladsp.sh — Install CamillaDSP (per-source parametric EQ)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Repo: HEnquist/camilladsp
# Pinned release asset, verified by sha256 before it is installed. The daemon
# finds the binary at /usr/local/bin/camilladsp; without it, EQ settings are
# stored but not applied and audio is bridged with alsaloop as before.

set -euo pipefail

step "56 · CamillaDSP (per-source EQ)"

_name="camilladsp"
_version="v4.1.3"
_asset="camilladsp-linux-aarch64.tar.gz"
_sha256="d9a17092923ebfe5d20a770c6b6a7eb2268f9700f999bf604b9db09f518aca5a"
_url="https://github.com/HEnquist/camilladsp/releases/download/${_version}/${_asset}"
_bin="$INSTALL_PREFIX/bin/camilladsp"

# The pinned checksum is for the aarch64 asset only.
if [[ "$ARCH" != "aarch64" ]]; then
    warn "camilladsp: no pinned release for arch '$ARCH' — skipping (EQ will be unavailable)"
    record_skip "camilladsp (unsupported arch)"
    return 0
fi

# Installed binary reports "CamillaDSP 4.1.3" for -V.
_installed_ver="$(read_installed_version "$_name")"
_running_ver=""
if [[ -x "$_bin" ]]; then
    _running_ver="$("$_bin" -V 2>/dev/null | head -1 || true)"
fi

if [[ "$_installed_ver" == "$_version" ]] && [[ "$_running_ver" == *"${_version#v}"* ]]; then
    skip "camilladsp $_version already installed"
    record_skip "camilladsp"
else
    log "Installing camilladsp ${_version} (installed: '${_installed_ver:-none}', binary reports: '${_running_ver:-none}')"

    _tmp_dir="$BUILD_DIR/camilladsp-download"
    rm -rf "$_tmp_dir"
    mkdir -p "$_tmp_dir"

    curl -fsSL -o "$_tmp_dir/$_asset" "$_url"

    _got_sha="$(sha256sum "$_tmp_dir/$_asset" | cut -d' ' -f1)"
    if [[ "$_got_sha" != "$_sha256" ]]; then
        error "camilladsp: checksum mismatch for $_url"
        error "  expected $_sha256"
        error "  got      $_got_sha"
        rm -rf "$_tmp_dir"
        exit 1
    fi
    log "camilladsp: sha256 verified"

    mkdir -p "$_tmp_dir/extract"
    tar -xzf "$_tmp_dir/$_asset" -C "$_tmp_dir/extract"
    _extracted_bin="$(find "$_tmp_dir/extract" -name camilladsp -type f | head -1)"
    if [[ -z "$_extracted_bin" ]]; then
        error "camilladsp binary not found in $_asset"
        exit 1
    fi
    install -m 0755 "$_extracted_bin" "$_bin"
    rm -rf "$_tmp_dir"

    log "camilladsp installed to $_bin ($("$_bin" -V 2>/dev/null | head -1 || echo 'version unknown'))"
    write_installed_version "$_name" "$_version"
    record_done "camilladsp ${_version}"
fi
