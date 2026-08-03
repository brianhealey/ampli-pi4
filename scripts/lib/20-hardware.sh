#!/usr/bin/env bash
# 20-hardware.sh — Raspberry Pi hardware configuration
# Sourced by setup.sh. Requires common.sh to be sourced first.
#
# Based on the AmpliPi reference config/boot_config.txt.
#
# Key facts from the real hardware:
#   - I2S is on GPIO18-21 by DEFAULT, same as SPI1.
#   - dtoverlay=i2s-gpio28-31 moves I2S to GPIO28-31, freeing GPIO18-21 for SPI1.
#   - hifiberry-dac overlay (NOT dacplus) drives the PCM5102A on the preamp board.
#   - spi1-2cs enables SPI1 with CE0=GPIO18, CE1=GPIO17.
#   - GPIO4 = preamp nRESET (output, default HIGH = running).
#   - GPIO5 = preamp BOOT0 (output, default LOW = run firmware, not bootloader).
#   - core_freq lock needed for stable SPI clock on BCM2837; harmless on BCM2711.

set -euo pipefail

step "20 · Hardware configuration"

_boot_cfg="$(boot_config)"
log "Boot config: $_boot_cfg"

# ── Boot config entries ──────────────────────────────────────────────────────
step "Boot config overlays"

# Disable onboard headphone audio jack (replaced by HiFiBerry DAC)
boot_add "dtparam=audio=off"

# I²C bus for preamp control (100 kHz)
boot_add "dtparam=i2c_arm=on"
boot_add "dtparam=i2c_arm_baudrate=100000"

# I2S audio interface
boot_add "dtparam=i2s=on"

# Drop the KMS display stack. This is a headless audio controller — the front
# panel is an SPI TFT driven directly (see cmd/amplipi-display), not through
# DRM — so nothing here uses vc4. Left in place it finds no display at boot,
# logs "Cannot find any crtc or sizes", and then trips four WARN_ON assertions
# in vc4_atomic_commit_tail with full stack traces, which look alarming in
# dmesg and bury real problems.
# max_framebuffers is meaningless once the overlay is gone.
# NOTE: this leaves the unit with no HDMI output at all.
boot_remove "dtoverlay=vc4-kms-v3d"
boot_remove "max_framebuffers="

# With vc4 gone, this would leave no HDMI output whatsoever — not even a
# firmware console. Removing it lets the firmware set up a legacy framebuffer,
# so a monitor still shows boot output if the unit ever needs debugging at the
# console, without bringing back the KMS stack or its warnings.
boot_remove "disable_fw_kms_setup"

# Hardware UART on /dev/serial0 for STM32 preamp address assignment
boot_add "enable_uart=1"

# Move Bluetooth to mini-UART so hardware UART is free
# (CM4S has no BT at all, but this overlay is harmless and keeps config portable)
boot_add "dtoverlay=disable-bt"

# HiFiBerry DAC (PCM5102A on preamp carrier board — NOT dacplus)
# Then move I2S from GPIO18-21 to GPIO28-31, freeing GPIO18-21 for SPI1.
boot_replace "dtoverlay=hifiberry-dac" "dtoverlay=hifiberry-dac"
boot_add "dtoverlay=i2s-gpio28-31"

# Preamp GPIO lines (set via boot config so they're correct before firmware starts)
#   GPIO4: nRESET — active-low reset, default HIGH (preamp running)
#   GPIO5: BOOT0  — LOW = run firmware, HIGH = enter STM32 bootloader
boot_add "gpio=4=op,dh"
boot_add "gpio=5=op,dl"

# Generous GPU RAM to silence VCHI/DRM errors and enable HW video decode
boot_replace "gpu_mem=" "gpu_mem=128"

# Lock core_freq for stable SPI clock speed (needed on BCM2837; harmless on BCM2711)
boot_add "core_freq=400"
boot_add "core_freq_min=400"

# ── SPI buses ────────────────────────────────────────────────────────────────
# DISPLAY_MODE is set by check_pi() in common.sh:
#
#   "test-board"    → Pi 4/3/5 standard 40-pin header
#     SPI0 only: SCLK=GPIO11, MOSI=GPIO10, MISO=GPIO9, CE0=GPIO8, DC=GPIO25
#
#   "carrier-board" → AmpliPi CM3+/CM4S carrier board
#     SPI1: peak-detect ADC on the preamp board
#           SCLK=GPIO21, MOSI=GPIO20, MISO=GPIO19, CE0=GPIO18, CE1=GPIO17
#           (GPIO18-21 are free because i2s-gpio28-31 moved I2S to GPIO28-31)
#     J16 front panel display — GPIO40-45, per controller_board_3a.pdf:
#           MISO=GPIO40, MOSI=GPIO41, SCLK=GPIO42, TFT CE=GPIO44, touch CE=GPIO45,
#           DC=GPIO39, RESET=GPIO38, backlight=PWM0/GPIO12
#           CM3+ reaches these as SPI2; CM4S as SPI0 (see the branch below)

step "SPI configuration (detected mode: $DISPLAY_MODE)"

boot_add "dtparam=spi=on"   # SPI0 always enabled

if [[ "$DISPLAY_MODE" == "carrier-board" ]]; then
    # SPI1 with 2 chip selects: CE0=GPIO18, CE1=GPIO17.
    # GPIO18-21 are free because I2S was moved to GPIO28-31 above.
    boot_replace "dtoverlay=spi1-" "dtoverlay=spi1-2cs"

    if [[ "$PI_MODEL" == "Pi CM4S" ]]; then
        # The front panel display (J16) is wired to GPIO40-45. On BCM2837 those
        # pins are SPI2 in ALT4, which is what the board was designed against.
        # On BCM2711 the same ALT4 is SPI0:
        #   GPIO40=SPI0_MISO  41=SPI0_MOSI  42=SPI0_SCLK  44=SPI0_CE1_N
        # so spi2-2cs binds a controller whose signals never reach the header —
        # it claims GPIO43/44 as plain GPIOs and drives no clock or data. That
        # is why the panel showed a white screen with a working backlight.
        #
        # Instead: route SPI0 onto the display pins and relocate its CS1 to
        # GPIO44 (J16 pin 2). SPI0 keeps its normal GPIO7-11 group as well;
        # both are driven, which is harmless because only the asserted chip
        # select decides who listens.
        boot_remove "dtoverlay=spi2-2cs"
        boot_replace "dtoverlay=spi0-2cs" "dtoverlay=spi0-2cs,cs1_pin=44"
        boot_add "gpio=40,41,42=a4"
        log "CM4S: front panel TFT on SPI0 (CS1→GPIO44, data on GPIO40-42 ALT4)"
    else
        boot_add "dtoverlay=spi2-2cs"   # TFT + eInk on SPI2, CE1/CE2 — CM3+ only
        log "Carrier-board SPI overlays added (SPI1 + SPI2)"
    fi
else
    skip "SPI1/SPI2 overlays (not needed for $DISPLAY_MODE mode — SPI0 only)"
fi

# ── Kernel modules ───────────────────────────────────────────────────────────
step "Kernel modules"

# i2c-dev: userspace access to I²C bus
ensure_line_in_file "i2c-dev" "/etc/modules"
log "i2c-dev in /etc/modules"

# spidev: userspace access to SPI buses
ensure_line_in_file "spidev" "/etc/modules"
log "spidev in /etc/modules"

# ── CPU performance governor ─────────────────────────────────────────────────
step "CPU performance governor"

_cpu_svc="/etc/systemd/system/cpu-performance.service"
_cpu_svc_content="[Unit]
Description=Set CPU scaling governor to performance
DefaultDependencies=no
After=sysinit.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'for f in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do echo performance > \$f; done'
ExecStop=/bin/sh -c 'for f in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do echo ondemand > \$f; done'

[Install]
WantedBy=multi-user.target"

if [[ -f "$_cpu_svc" ]] && [[ "$(cat "$_cpu_svc")" == "$_cpu_svc_content" ]]; then
    skip "cpu-performance.service (unchanged)"
    record_skip "cpu-performance.service"
else
    echo "$_cpu_svc_content" > "$_cpu_svc"
    log "wrote $_cpu_svc"
fi

# Enable and start (idempotent)
if systemctl is-enabled --quiet cpu-performance.service 2>/dev/null; then
    skip "cpu-performance.service (already enabled)"
    record_skip "cpu-performance.service enable"
else
    systemctl daemon-reload
    systemctl enable cpu-performance.service
    systemctl start cpu-performance.service || warn "cpu-performance.service start failed (may need reboot)"
    log "cpu-performance.service enabled and started"
    record_done "cpu-performance.service"
fi

# ── Summary ──────────────────────────────────────────────────────────────────
if (( REBOOT_NEEDED )); then
    warn "Boot config changed — reboot required for hardware changes to take effect"
    record_done "hardware overlays (boot config modified — reboot needed)"
else
    record_skip "hardware overlays (boot config unchanged)"
fi
