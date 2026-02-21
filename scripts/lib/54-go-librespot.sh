#!/usr/bin/env bash
# 54-go-librespot.sh — Install go-librespot (Spotify Connect)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Repo: devgianlu/go-librespot
# Downloads pre-built binary for the correct arch — no build required.

set -euo pipefail

step "54 · go-librespot (Spotify Connect)"

_name="go-librespot"
_repo="devgianlu/go-librespot"
_bin="$INSTALL_PREFIX/bin/go-librespot"
_conf="/etc/go-librespot.yml"

# ── Map arch to asset name ────────────────────────────────────────────────────
case "$ARCH" in
    aarch64) _asset_suffix="linux_arm64" ;;
    armv7l)  _asset_suffix="linux_arm"   ;;
    *)
        warn "go-librespot: unsupported arch '$ARCH' — skipping"
        record_skip "go-librespot (unsupported arch)"
        return 0
        ;;
esac

# ── Get latest release info ───────────────────────────────────────────────────
_latest_tag="$(latest_github_tag "$_repo")"
log "Latest upstream tag: $_latest_tag"

_installed_ver="$(read_installed_version "$_name")"

if [[ -n "$_installed_ver" ]] && [[ "$_installed_ver" == "$_latest_tag" ]] && \
   [[ -x "$_bin" ]]; then
    skip "go-librespot $_installed_ver already installed"
    record_skip "go-librespot"
else
    log "Installing go-librespot ${_latest_tag} (installed: '${_installed_ver:-none}')"

    # Build the download URL from GitHub release assets
    # Asset naming: go-librespot_linux_arm64 or go-librespot_linux_arm
    # Releases may use a tarball: go-librespot_<tag>_linux_arm64.tar.gz
    # Try tarball first, then bare binary
    _ver_clean="${_latest_tag#v}"  # strip leading 'v' for some repos
    _tarball_url="https://github.com/${_repo}/releases/download/${_latest_tag}/go-librespot_${_ver_clean}_${_asset_suffix}.tar.gz"
    _bin_url="https://github.com/${_repo}/releases/download/${_latest_tag}/go-librespot_${_asset_suffix}"

    _tmp_dir="$BUILD_DIR/go-librespot-download"
    mkdir -p "$_tmp_dir"

    # Try tarball
    _downloaded=0
    if curl -fsSL --head "$_tarball_url" &>/dev/null; then
        log "Downloading tarball: $_tarball_url"
        curl -fsSL -o "$_tmp_dir/go-librespot.tar.gz" "$_tarball_url"
        tar -xzf "$_tmp_dir/go-librespot.tar.gz" -C "$_tmp_dir"
        # Find the binary inside the tarball
        _extracted_bin="$(find "$_tmp_dir" -name "go-librespot" -type f | head -1)"
        if [[ -z "$_extracted_bin" ]]; then
            error "go-librespot binary not found in tarball"
            exit 1
        fi
        install -m 0755 "$_extracted_bin" "$_bin"
        _downloaded=1
    elif curl -fsSL --head "$_bin_url" &>/dev/null; then
        log "Downloading binary: $_bin_url"
        curl -fsSL -o "$_bin" "$_bin_url"
        chmod 0755 "$_bin"
        _downloaded=1
    fi

    if (( ! _downloaded )); then
        error "go-librespot: could not download binary for ${_latest_tag}/${_asset_suffix}"
        error "  Tried: $_tarball_url"
        error "  Tried: $_bin_url"
        exit 1
    fi

    log "go-librespot installed to $_bin"
    write_installed_version "$_name" "$_latest_tag"
    record_done "go-librespot ${_latest_tag}"
fi

# ── Default config (never overwrite existing) ─────────────────────────────────
step "go-librespot default config"
if [[ -f "$_conf" ]]; then
    skip "$_conf (already exists)"
    record_skip "go-librespot.yml"
else
    cat > "$_conf" <<'EOF'
# go-librespot configuration
# See: https://github.com/devgianlu/go-librespot/blob/master/README.md
device_name: "AmpliPi"
device_type: "speaker"
audio_device: "lb2"
audio_format: "s16le"
EOF
    log "wrote $_conf"
    record_done "go-librespot.yml"
fi
