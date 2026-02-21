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
#     SPI1: SCLK=GPIO21, MOSI=GPIO20, MISO=GPIO19, CE0=GPIO18, CE1=GPIO17, DC=GPIO39
#           (GPIO18-21 are free because i2s-gpio28-31 moved I2S to GPIO28-31)
#     SPI2: eInk Waveshare 2.13" (CE1, bus=2 device=1) — CM3+ only, not CM4S

step "SPI configuration (detected mode: $DISPLAY_MODE)"

boot_add "dtparam=spi=on"   # SPI0 always enabled

if [[ "$DISPLAY_MODE" == "carrier-board" ]]; then
    # SPI1 with 2 chip selects: CE0=GPIO18, CE1=GPIO17.
    # GPIO18-21 are free because I2S was moved to GPIO28-31 above.
    boot_replace "dtoverlay=spi1-" "dtoverlay=spi1-2cs"

    if [[ "$PI_MODEL" == "Pi CM4S" ]]; then
        # BCM2711 (CM4S) has no SPI2 on GPIO40-45.
        # (GPIO40-44 ALT3 = SPI0, not SPI2; spi2-2cs overlay fails probe.)
        # eInk display (Waveshare 2.13") uses spidev.open(2,1) → will not work on CM4S.
        # TFT display (SPI1) is unaffected.
        warn "CM4S detected: skipping spi2-2cs (BCM2711 has no SPI2 on GPIO40-45)"
        warn "eInk display will NOT work on CM4S. TFT display (SPI1) is unaffected."
        record_skip "spi2-2cs (CM4S/BCM2711 incompatible — SPI0 is on GPIO40-44 ALT3, not SPI2)"
    else
        boot_add "dtoverlay=spi2-2cs"   # eInk on SPI2, CE1 — CM3+ only
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
