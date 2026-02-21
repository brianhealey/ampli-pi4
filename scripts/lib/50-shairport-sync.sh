#!/usr/bin/env bash
# 50-shairport-sync.sh — Build and install shairport-sync (AirPlay 1 + 2)
#                         and its companion nqptp (required for AirPlay 2)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Build order:
#   1. nqptp   (mikebrady/nqptp)     — AirPlay 2 PTP timing companion
#   2. shairport-sync (mikebrady/shairport-sync) — AirPlay 1 + 2 receiver
#
# shairport-sync build flags:
#   --with-alsa --with-avahi --with-ssl=openssl --with-soxr
#   --with-airplay-2 --sysconfdir=/etc --with-systemd
# Pi 4 optimised: CFLAGS=$MARCH_FLAGS

set -euo pipefail

step "50 · nqptp (AirPlay 2 PTP companion)"

_nqptp_repo="mikebrady/nqptp"
_nqptp_src="$BUILD_DIR/nqptp"

_nqptp_latest="$(latest_github_tag "$_nqptp_repo")"
log "Latest nqptp tag: $_nqptp_latest"
_nqptp_installed="$(read_installed_version "nqptp")"

if [[ -n "$_nqptp_installed" ]] && [[ "$_nqptp_installed" == "$_nqptp_latest" ]] && \
   command -v nqptp &>/dev/null; then
    skip "nqptp $_nqptp_installed already installed"
    record_skip "nqptp"
else
    log "Building nqptp ${_nqptp_latest} (installed: '${_nqptp_installed:-none}')"
    git_checkout_tag "https://github.com/${_nqptp_repo}.git" "$_nqptp_src" "$_nqptp_latest"
    cd "$_nqptp_src"
    autoreconf -i -f >/dev/null 2>&1
    ./configure --with-systemd-startup
    make -j"$CORES"
    make install
    write_installed_version "nqptp" "$_nqptp_latest"
    record_done "nqptp ${_nqptp_latest}"
fi

# Enable nqptp systemd service (must run as root for PTP access)
if systemctl is-enabled --quiet nqptp 2>/dev/null; then
    skip "nqptp.service (already enabled)"
    record_skip "nqptp.service"
else
    systemctl daemon-reload
    systemctl enable nqptp
    systemctl start nqptp || warn "nqptp start failed — will retry after reboot"
    log "nqptp.service enabled"
    record_done "nqptp.service"
fi

step "50 · shairport-sync (AirPlay)"

_name="shairport-sync"
_repo="mikebrady/shairport-sync"
_src_dir="$BUILD_DIR/shairport-sync"
_conf="/etc/${_name}.conf"

# ── Version check ─────────────────────────────────────────────────────────────
step "Checking shairport-sync version"
_latest_tag="$(latest_github_tag "$_repo")"
log "Latest upstream tag: $_latest_tag"

_installed_ver="$(read_installed_version "$_name")"

if [[ -n "$_installed_ver" ]] && [[ "$_installed_ver" == "$_latest_tag" ]] && \
   command -v shairport-sync &>/dev/null; then
    skip "shairport-sync $_installed_ver already installed"
    record_skip "shairport-sync"
else
    log "Installing shairport-sync ${_latest_tag} (installed: '${_installed_ver:-none}')"

    # ── Clone / update source ─────────────────────────────────────────────────
    git_checkout_tag "https://github.com/${_repo}.git" "$_src_dir" "$_latest_tag"
    cd "$_src_dir"

    # ── Build ─────────────────────────────────────────────────────────────────
    autoreconf -i -f >/dev/null 2>&1
    CFLAGS="$MARCH_FLAGS" ./configure \
        --with-alsa \
        --with-avahi \
        --with-ssl=openssl \
        --with-soxr \
        --with-airplay-2 \
        --sysconfdir=/etc \
        --with-systemd \
        --prefix="$INSTALL_PREFIX"

    make -j"$CORES"
    make install
    log "shairport-sync built and installed"

    write_installed_version "$_name" "$_latest_tag"
    record_done "shairport-sync ${_latest_tag}"
fi

# ── Default config (never overwrite existing) ─────────────────────────────────
step "shairport-sync default config"
if [[ -f "$_conf" ]]; then
    skip "$_conf (already exists)"
    record_skip "shairport-sync.conf"
else
    cat > "$_conf" <<'EOF'
// AmpliPi shairport-sync configuration
// See: https://github.com/mikebrady/shairport-sync/blob/master/CONFIGURATION%20REFERENCE.md
general = {
  name = "AmpliPi";
  port = 5000;
  drift_tolerance_in_seconds = 0.002;
  resync_threshold_in_seconds = 0.05;
  log_verbosity = 0;
};
alsa = {
  output_device = "lb0";
  mixer_control_name = "Master";
};
EOF
    log "wrote $_conf"
    record_done "shairport-sync.conf"
fi

# ── Create user ───────────────────────────────────────────────────────────────
step "shairport-sync user"
if ! id shairport-sync >/dev/null 2>&1; then
    useradd -r -M -g audio -s /usr/sbin/nologin shairport-sync
    log "created shairport-sync user"
    record_done "shairport-sync user"
else
    skip "shairport-sync user exists"
    record_skip "shairport-sync user"
fi

# ── Install systemd service ───────────────────────────────────────────────────
step "shairport-sync systemd service"

_svc="/lib/systemd/system/shairport-sync.service"
if [[ ! -f "$_svc" ]]; then
    cat > "$_svc" <<'EOF'
[Unit]
Description=Shairport Sync - AirPlay Audio Receiver
After=sound.target
Wants=network-online.target
After=network.target network-online.target
After=avahi-daemon.service
Requires=avahi-daemon.service
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
ExecStart=/usr/local/bin/shairport-sync --log-to-syslog
User=shairport-sync
Group=audio
LimitRTPRIO=5
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF
    log "installed $_svc"
    record_done "shairport-sync.service file"
fi

if systemctl is-enabled --quiet shairport-sync 2>/dev/null; then
    skip "shairport-sync.service (already enabled)"
    record_skip "shairport-sync.service"
else
    systemctl daemon-reload
    systemctl enable shairport-sync
    log "shairport-sync.service enabled"
    record_done "shairport-sync.service"
fi
