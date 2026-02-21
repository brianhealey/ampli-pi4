#!/usr/bin/env bash
# 40-deps.sh — Build and runtime dependencies (apt)
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Installs all libraries needed to build:
#   shairport-sync, squeezelite, pianobar, gmrender-resurrect,
#   go-librespot, bluez-alsa
# and runtime dependencies for VLC, RTL-SDR, etc.

set -euo pipefail

step "40 · Build dependencies"

# apt is idempotent — install everything in one shot for speed.
# Missing packages are installed; already-present ones are a no-op.

_deps=(
    # ── Audio codec libraries ────────────────────────────────────────────────
    libasound2-dev          # ALSA (shairport-sync, squeezelite, bluez-alsa)
    libpulse-dev            # PulseAudio headers
    libsoxr-dev             # Sample-rate conversion (shairport-sync)
    libflac-dev             # FLAC (squeezelite)
    libvorbis-dev           # Ogg Vorbis (squeezelite)
    libmpg123-dev           # MP3 (squeezelite)
    libfaad-dev             # AAC (bluez-alsa, squeezelite)
    libopus-dev             # Opus (squeezelite, bluez-alsa)

    # ── Avahi / mDNS ────────────────────────────────────────────────────────
    libavahi-client-dev     # Avahi client (shairport-sync, squeezelite)
    libavahi-common-dev     # Avahi common

    # ── TLS / crypto ────────────────────────────────────────────────────────
    libssl-dev              # OpenSSL (shairport-sync AirPlay 2)
    libsodium-dev           # libsodium (shairport-sync AirPlay 2)

    # ── Misc audio helpers ───────────────────────────────────────────────────
    libdaemon-dev           # daemon() helper (shairport-sync)
    libplist-dev            # plist library (shairport-sync AirPlay 2)
    libplist-utils          # plistutil command-line tool (required by shairport-sync configure)
    xxd                     # hex dump tool (required by shairport-sync AirPlay 2 build)
    libgcrypt20-dev         # libgcrypt (required by shairport-sync AirPlay 2 build)

    # ── FFmpeg ──────────────────────────────────────────────────────────────
    libavutil-dev           # FFmpeg utils (squeezelite --FFMPEG=1)
    libavcodec-dev          # FFmpeg codecs
    libavformat-dev         # FFmpeg formats
    libswresample-dev       # FFmpeg resampler
    libavfilter-dev         # FFmpeg filter (pianobar)

    # ── Config / CLI parsing ────────────────────────────────────────────────
    libconfig-dev           # libconfig (shairport-sync config file)
    libpopt-dev             # popt (shairport-sync, pianobar)
    libcurl4-openssl-dev    # libcurl (pianobar)

    # ── GLib / D-Bus ────────────────────────────────────────────────────────
    libglib2.0-dev          # GLib (bluez-alsa, gmrender-resurrect)
    libdbus-1-dev           # D-Bus (bluez-alsa)

    # ── GStreamer (gmrender-resurrect) ───────────────────────────────────────
    gstreamer1.0-tools
    gstreamer1.0-plugins-base
    gstreamer1.0-plugins-good
    gstreamer1.0-plugins-bad
    gstreamer1.0-alsa
    libgstreamer1.0-dev
    libgstreamer-plugins-base1.0-dev

    # ── Bluetooth (bluez-alsa) ───────────────────────────────────────────────
    bluetooth
    bluez
    bluez-tools
    libbluetooth-dev        # Bluetooth headers (bluez-alsa)
    libsbc-dev              # SBC codec for A2DP (bluez-alsa)

    # ── UPnP/DLNA (gmrender-resurrect) ──────────────────────────────────────
    libgupnp-1.2-dev        # GUPnP UPnP stack (gmrender-resurrect)
    libgupnp-av-1.0-dev     # GUPnP AV profile (gmrender-resurrect)
    libgssdp-1.2-dev        # SSDP (GUPnP dependency)

    # ── Pandora (pianobar) ───────────────────────────────────────────────────
    libao-dev               # Audio output (pianobar)
    libmad0-dev             # MP3 decoding (pianobar) — libmad-dev renamed in Debian trixie
    libjson-glib-dev        # JSON-GLib (pianobar uses json-glib-1.0, not json-c)

    # ── Python ──────────────────────────────────────────────────────────────
    python3-pip
    python3-venv

    # ── RTL-SDR (FM tuner) ──────────────────────────────────────────────────
    rtl-sdr

    # ── VLC (fallback player) ───────────────────────────────────────────────
    vlc

    # ── ALSA utilities ──────────────────────────────────────────────────────
    alsa-utils              # alsaloop, aplay, amixer, etc.

    # ── autotools for builds ────────────────────────────────────────────────
    autoconf
    automake
    libtool
)

apt-get install -y --no-install-recommends "${_deps[@]}"
log "all build dependencies installed"
record_done "build dependencies (${#_deps[@]} packages)"
