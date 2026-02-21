#!/usr/bin/env bash
# 30-alsa.sh — ALSA configuration for AmpliPi
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Based on the AmpliPi reference config/asound.conf.
#
# Key facts:
#   - snd-aloop creates 6 separate loopback CARDS (Loopback, Loopback_1 … Loopback_5),
#     NOT one card with 6 substreams.
#   - asound.conf references cards by name (hw:sndrpihifiberry, hw:Loopback, etc.)
#     so card index assignments don't matter as long as names are correct.
#   - ch0  = HiFiBerry DAC (card sndrpihifiberry) — main audio output
#   - lb0-lb5 = write (playback dmix) side of each loopback — streams write here
#   - lb0c-lb5c = capture side of each loopback — Go code reads from here for routing
#   - ch1-ch3 = USB 8-ch DAC (CM6206, card cmedia8chint) — optional, gracefully absent

set -euo pipefail

step "30 · ALSA configuration"

# ── Install ALSA packages ─────────────────────────────────────────────────────
apt-get install -y --no-install-recommends alsa-utils libasound2-dev
log "ALSA packages installed"

# ── snd-aloop module load ─────────────────────────────────────────────────────
step "snd-aloop module"

_modload_conf="/etc/modules-load.d/amplipi.conf"
if grep -qxF "snd-aloop" "$_modload_conf" 2>/dev/null; then
    skip "snd-aloop in $_modload_conf"
else
    ensure_line_in_file "snd-aloop" "$_modload_conf"
    log "snd-aloop added to $_modload_conf"
    record_done "snd-aloop modules-load"
fi

# ── snd-aloop module options ──────────────────────────────────────────────────
# 6 separate loopback CARDS, each with 2 devices (device 0 write, device 1 read).
# NOTE: The Linux kernel strips underscores from ALSA card IDs.
#   Specifying id=Loopback_1 results in actual card ID: Loopback1
#   So we use names WITHOUT underscores to match the kernel's output.
# pcm_substreams must be specified per-card (kernel default=8 if omitted).
_modprobe_conf="/etc/modprobe.d/amplipi-alsa.conf"
_modprobe_content="options snd-aloop enable=1,1,1,1,1,1 index=2,3,4,5,6,7 id=Loopback,Loopback1,Loopback2,Loopback3,Loopback4,Loopback5 pcm_substreams=2,2,2,2,2,2"

if write_if_changed "$_modprobe_conf" "$_modprobe_content"; then
    record_done "snd-aloop modprobe options"
else
    record_skip "snd-aloop modprobe options"
fi

# ── Reload snd-aloop if it's already loaded and options changed ───────────────
if lsmod | grep -q "^snd_aloop"; then
    # Check if the current card names match what we need
    if ! aplay -l 2>/dev/null | grep -q "Loopback_1"; then
        log "snd-aloop loaded but wrong config — reloading"
        modprobe -r snd-aloop 2>/dev/null || warn "could not remove snd-aloop (in use?)"
        modprobe snd-aloop || warn "modprobe snd-aloop failed"
        log "snd-aloop reloaded"
        record_done "snd-aloop reload"
    else
        skip "snd-aloop already loaded with correct config"
    fi
else
    modprobe snd-aloop || warn "modprobe snd-aloop failed (may need reboot)"
    log "snd-aloop loaded"
    record_done "snd-aloop modprobe"
fi

# ── /etc/asound.conf ─────────────────────────────────────────────────────────
step "ALSA device config (/etc/asound.conf)"

# Adapted from AmpliPi config/asound.conf.
# ch0 routes through HiFiBerry with softvol attenuation (-7.1 dB cap).
# lb0-lb11 provide read/write access to the 6 loopback sources.
# ch1-ch3 route to USB 8-ch DAC (CM6206); absent hardware is gracefully ignored.
_asound_conf="/etc/asound.conf"
_asound_content='# AmpliPi ALSA configuration — adapted from config/asound.conf

# Default output: source 0 (HiFiBerry)
pcm.!default {
    type            plug
    slave.pcm       "ch0"
}

# ── ch0: HiFiBerry DAC output (preamp source 0) ───────────────────────────────
pcm.ch0 {
    type            plug
    slave.pcm       "ch0_softvol"
    slave.channels  2
}

# Attenuate to -7.1 dB to avoid clipping on older preamp op-amps (Preamp < Rev4).
# Kept for backwards compatibility.
pcm.ch0_softvol {
    type            softvol
    slave.pcm       "ch0_dmix"
    control.name    "Ch0 Volume"
    control.card    0
    max_dB          -7.1
    resolution      256
}

pcm.ch0_dmix {
    type            dmix
    ipc_key         2867
    ipc_perm        0666
    slave {
        pcm         "hw:sndrpihifiberry,0"
        period_time 0
        period_size 1024
        buffer_size 8192
        channels    2
    }
}

ctl.ch0 {
    type            hw
    card            sndrpihifiberry
}

# ── USB 8-ch DAC (CM6206) — sources 1-3 on AmpliPi v2 ────────────────────────
# Gracefully absent if no USB DAC is connected.
pcm.usb71 {
    type            hw
    card            cmedia8chint
}

ctl.usb71 {
    type            hw
    card            cmedia8chint
}

pcm.dmixer {
    type            dmix
    ipc_key         1024
    ipc_perm        0666
    slave {
        pcm         "usb71"
        period_time 0
        period_size 1024
        buffer_size 4096
        channels    8
    }
    bindings { 0 0; 1 1; 2 2; 3 3; 4 4; 5 5; 6 6; 7 7; }
}

pcm.ch1 {
    type                plug
    slave.pcm {
        type            plug
        slave.pcm       "dmixer"
        slave.channels  8
        ttable.0.6      0.64
        ttable.1.7      0.64
    }
    slave.channels      2
}

pcm.ch2 {
    type                plug
    slave.pcm {
        type            plug
        slave.pcm       "dmixer"
        slave.channels  8
        ttable.0.0      0.64
        ttable.1.1      0.64
    }
    slave.channels      2
}

pcm.ch3 {
    type                plug
    slave.pcm {
        type            plug
        slave.pcm       "dmixer"
        slave.channels  8
        ttable.0.4      0.64
        ttable.1.5      0.64
    }
    slave.channels      2
}

# ── Loopback sources — 6 cards (Loopback .. Loopback_5) ─────────────────────
# Each card has 2 devices: device 0 (write side) and device 1 (read side).
# Streams write to lbNp (plug, forces 48kHz S16_LE).
# Go routing code reads from lbN / lbNc (dmix / plug).

# -- Playback sinks (stream processes write here at forced 48kHz) --
pcm.lb0p {
    type plug
    slave { pcm "hw:Loopback,1"; rate 48000; format S16_LE; }
}
pcm.lb1p {
    type plug
    slave { pcm "hw:Loopback1,1"; rate 48000; format S16_LE; }
}
pcm.lb2p {
    type plug
    slave { pcm "hw:Loopback2,1"; rate 48000; format S16_LE; }
}
pcm.lb3p {
    type plug
    slave { pcm "hw:Loopback3,1"; rate 48000; format S16_LE; }
}
pcm.lb4p {
    type plug
    slave { pcm "hw:Loopback4,1"; rate 48000; format S16_LE; }
}
pcm.lb5p {
    type plug
    slave { pcm "hw:Loopback5,1"; rate 48000; format S16_LE; }
}

pcm.lb6p {
    type plug
    slave { pcm "hw:Loopback,0"; rate 48000; format S16_LE; }
}
pcm.lb7p {
    type plug
    slave { pcm "hw:Loopback1,0"; rate 48000; format S16_LE; }
}
pcm.lb8p {
    type plug
    slave { pcm "hw:Loopback2,0"; rate 48000; format S16_LE; }
}
pcm.lb9p {
    type plug
    slave { pcm "hw:Loopback3,0"; rate 48000; format S16_LE; }
}
pcm.lb10p {
    type plug
    slave { pcm "hw:Loopback4,0"; rate 48000; format S16_LE; }
}
pcm.lb11p {
    type plug
    slave { pcm "hw:Loopback5,0"; rate 48000; format S16_LE; }
}

# -- Capture sources (Go routing code reads from these) --
pcm.lb0 {
    type dmix
    ipc_key 1028; ipc_perm 0666
    slave { pcm "hw:Loopback,0"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb0c { type plug; slave.pcm "lb0"; }

pcm.lb1 {
    type dmix
    ipc_key 1029; ipc_perm 0666
    slave { pcm "hw:Loopback1,0"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb1c { type plug; slave.pcm "lb1"; }

pcm.lb2 {
    type dmix
    ipc_key 1030; ipc_perm 0666
    slave { pcm "hw:Loopback2,0"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb2c { type plug; slave.pcm "lb2"; }

pcm.lb3 {
    type dmix
    ipc_key 1031; ipc_perm 0666
    slave { pcm "hw:Loopback3,0"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb3c { type plug; slave.pcm "lb3"; }

pcm.lb4 {
    type dmix
    ipc_key 1032; ipc_perm 0666
    slave { pcm "hw:Loopback4,0"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb4c { type plug; slave.pcm "lb4"; }

pcm.lb5 {
    type dmix
    ipc_key 1033; ipc_perm 0666
    slave { pcm "hw:Loopback5,0"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb5c { type plug; slave.pcm "lb5"; }

pcm.lb6 {
    type dmix
    ipc_key 1034; ipc_perm 0666
    slave { pcm "hw:Loopback,1"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb6c { type plug; slave.pcm "lb6"; }

pcm.lb7 {
    type dmix
    ipc_key 1035; ipc_perm 0666
    slave { pcm "hw:Loopback1,1"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb7c { type plug; slave.pcm "lb7"; }

pcm.lb8 {
    type dmix
    ipc_key 1036; ipc_perm 0666
    slave { pcm "hw:Loopback2,1"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb8c { type plug; slave.pcm "lb8"; }

pcm.lb9 {
    type dmix
    ipc_key 1037; ipc_perm 0666
    slave { pcm "hw:Loopback3,1"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb9c { type plug; slave.pcm "lb9"; }

pcm.lb10 {
    type dmix
    ipc_key 1038; ipc_perm 0666
    slave { pcm "hw:Loopback4,1"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb10c { type plug; slave.pcm "lb10"; }

pcm.lb11 {
    type dmix
    ipc_key 1039; ipc_perm 0666
    slave { pcm "hw:Loopback5,1"; period_time 0; period_size 1024; buffer_size 4096; channels 2; }
}
pcm.lb11c { type plug; slave.pcm "lb11"; }
'

if write_if_changed "$_asound_conf" "$_asound_content"; then
    record_done "asound.conf"
else
    record_skip "asound.conf"
fi

# ── Save mixer state ──────────────────────────────────────────────────────────
alsactl store 2>/dev/null || warn "alsactl store failed (no sound cards yet? OK after reboot)"
log "ALSA mixer state saved"
