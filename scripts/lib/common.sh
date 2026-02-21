#!/usr/bin/env bash
# common.sh — Shared utilities for AmpliPi setup scripts
# Source this from every lib script. Do NOT execute directly.
# shellcheck disable=SC2034  # Variables may be used by sourcing scripts

set -euo pipefail

# ── Colour helpers ──────────────────────────────────────────────────────────
_CLR_GREEN='\033[0;32m'
_CLR_YELLOW='\033[1;33m'
_CLR_RED='\033[0;31m'
_CLR_CYAN='\033[0;36m'
_CLR_RESET='\033[0m'

log()   { echo -e "${_CLR_GREEN}[OK]${_CLR_RESET}   $*"; }
warn()  { echo -e "${_CLR_YELLOW}[WARN]${_CLR_RESET} $*"; }
error() { echo -e "${_CLR_RED}[ERR]${_CLR_RESET}  $*" >&2; }
step()  { echo -e "\n${_CLR_CYAN}==>${_CLR_RESET} $*"; }
skip()  { echo -e "${_CLR_YELLOW}[SKIP]${_CLR_RESET} $* — already done"; }

# ── Root check ───────────────────────────────────────────────────────────────
require_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This script must be run as root (use sudo)."
        exit 1
    fi
}

# ── Non-interactive apt (suppress TTY prompts and debconf frontend warnings) ──
export DEBIAN_FRONTEND=noninteractive
export DEBCONF_NOWARNINGS=yes

# ── Globals (overridable by environment) ─────────────────────────────────────
BUILD_DIR="${AMPLIPI_BUILD_DIR:-/tmp/amplipi-build}"
INSTALL_PREFIX="${AMPLIPI_INSTALL_PREFIX:-/usr/local}"

# CONFIG_DIR belongs to the invoking user, not root
if [[ -n "${SUDO_USER:-}" ]]; then
    _INVOKING_HOME="$(getent passwd "$SUDO_USER" | cut -d: -f6)"
else
    _INVOKING_HOME="$HOME"
fi
CONFIG_DIR="${AMPLIPI_CONFIG_DIR:-${_INVOKING_HOME}/.config/amplipi}"

# Version store directory (root-owned, used to track installed binary versions)
VERSION_DIR="/etc/amplipi/versions"

# Summary tracking — arrays of what was done/skipped
DONE_ITEMS=()
SKIPPED_ITEMS=()
REBOOT_NEEDED=0   # Set to 1 by 20-hardware.sh if boot config changed

# ── Pi detection ─────────────────────────────────────────────────────────────
ARCH=""
CORES=""
MARCH_FLAGS=""
PI_MODEL=""

# DISPLAY_MODE controls which SPI bus + GPIO pins the display uses:
#   "test-board"    — SPI0 (GPIO10/11/9, CE0=GPIO8, DC=GPIO25)
#                     Use this for Pi 4 / Pi 3 / Pi 5 on a standard 40-pin header.
#   "carrier-board" — SPI1 (GPIO20/21/19, CE=GPIO44, DC=GPIO39) + SPI2 for eInk
#                     Use this for the original AmpliPi CM3+ carrier board.
# Set by check_pi() from device-tree model; can be overridden by env var.
DISPLAY_MODE="${AMPLIPI_DISPLAY_MODE:-}"

check_pi() {
    ARCH="$(uname -m)"
    CORES="$(nproc)"

    if [[ -f /proc/device-tree/model ]]; then
        local model
        model="$(cat /proc/device-tree/model 2>/dev/null | tr -d '\0')"
        if [[ "$model" == *"Pi 4"* ]]; then
            PI_MODEL="Pi 4"
        elif [[ "$model" == *"Pi 5"* ]]; then
            PI_MODEL="Pi 5"
        elif [[ "$model" == *"Pi 3"* ]]; then
            PI_MODEL="Pi 3"
        elif [[ "$model" == *"Compute Module 4S"* ]]; then
            PI_MODEL="Pi CM4S"
        elif [[ "$model" == *"Compute Module 3"* ]] || [[ "$model" == *"CM3"* ]]; then
            PI_MODEL="Pi CM3"
        else
            PI_MODEL="unknown ($model)"
        fi
    else
        PI_MODEL="unknown"
    fi

    # Derive DISPLAY_MODE from Pi model (unless already set by environment)
    #
    # carrier-board: AmpliPi original carrier (SODIMM GPIO44 as CE, SPI1 for TFT, SPI2 for eInk)
    #   CM3+  (BCM2837): SPI1=GPIO18-21 alt4, SPI2=GPIO40-45 alt4 — both work
    #   CM4S  (BCM2711): SPI1=GPIO18-21 alt4 (same!), SPI2=NOT on GPIO40-45 (eInk broken)
    #     NOTE: On CM4S the eInk display (spi2-2cs) does not work — BCM2711 has no SPI2
    #     routing on GPIO40-45. Alt3 on GPIO40-44 maps to SPI0, not SPI2.
    #     If eInk is needed on CM4S, a different SPI bus + overlay is required.
    #
    # test-board: standard 40-pin header (SPI0, GPIO7-11, CE0=GPIO8, DC=GPIO25)
    if [[ -z "$DISPLAY_MODE" ]]; then
        case "$PI_MODEL" in
            "Pi CM3"|"Pi CM4S")
                # AmpliPi SODIMM carrier board — SPI1 + GPIO44 CE
                DISPLAY_MODE="carrier-board"
                ;;
            *)
                # Pi 4 / Pi 3 / Pi 5 / unknown — standard 40-pin header, SPI0
                DISPLAY_MODE="test-board"
                ;;
        esac
    fi

    case "$ARCH" in
        aarch64)
            # Pi 4 in 64-bit mode (Cortex-A72)
            MARCH_FLAGS="-march=armv8-a+crypto -mcpu=cortex-a72 -mtune=cortex-a72"
            ;;
        armv7l)
            # Pi 4 / Pi 3 in 32-bit mode (Cortex-A72 with NEON)
            MARCH_FLAGS="-march=armv7-a -mfpu=neon-fp-armv8 -mfloat-abi=hard -mcpu=cortex-a72"
            ;;
        *)
            warn "Unrecognised arch: $ARCH — compile flags left empty"
            MARCH_FLAGS=""
            ;;
    esac

    log "Platform: $PI_MODEL | Arch: $ARCH | Cores: $CORES | DisplayMode: $DISPLAY_MODE"
    log "MARCH_FLAGS: $MARCH_FLAGS"
}

# ── Boot config helpers ──────────────────────────────────────────────────────
boot_config() {
    if [[ -f /boot/firmware/config.txt ]]; then
        echo "/boot/firmware/config.txt"
    else
        echo "/boot/config.txt"
    fi
}

# boot_has <line>  — returns 0 if the line exists (exact, ignoring comments)
boot_has() {
    local line="$1"
    grep -qxF "$line" "$(boot_config)" 2>/dev/null
}

# boot_add <line>  — idempotently append to boot config; sets REBOOT_NEEDED=1
boot_add() {
    local line="$1"
    if boot_has "$line"; then
        skip "boot config: $line"
        return 0
    fi
    echo "$line" >> "$(boot_config)"
    log "boot config: added '$line'"
    REBOOT_NEEDED=1
}

# boot_replace <prefix> <new_line>  — replace any line matching prefix with new_line
# Use for overlays whose parameters may change between setup runs.
# Example: boot_replace "dtoverlay=spi1-1cs" "dtoverlay=spi1-1cs,cs0_pin=26"
boot_replace() {
    local prefix="$1"
    local new_line="$2"
    local cfg
    cfg="$(boot_config)"
    # Already exactly right?
    if grep -qxF "$new_line" "$cfg" 2>/dev/null; then
        skip "boot config: $new_line"
        return 0
    fi
    # Old version present? Replace it in-place.
    if grep -qF "$prefix" "$cfg" 2>/dev/null; then
        sed -i "s|^${prefix}.*|${new_line}|" "$cfg"
        log "boot config: replaced '${prefix}...' → '${new_line}'"
    else
        echo "$new_line" >> "$cfg"
        log "boot config: added '$new_line'"
    fi
    REBOOT_NEEDED=1
}

# ── GitHub helpers ───────────────────────────────────────────────────────────

# latest_github_tag <owner/repo>
# Prints the latest tag name (strips leading 'v' for shairport-sync compatibility).
latest_github_tag() {
    local repo="$1"
    local tag
    tag="$(curl -fsSL "https://api.github.com/repos/${repo}/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": "\(.*\)".*/\1/')"
    if [[ -z "$tag" ]]; then
        # Fall back to tags list (some repos don't do releases)
        tag="$(curl -fsSL "https://api.github.com/repos/${repo}/tags" \
            | grep '"name"' | head -1 | sed 's/.*"name": "\(.*\)".*/\1/')"
    fi
    echo "$tag"
}

# git_checkout_tag <url> <dir> <tag>
# Clones the repo if missing; updates and checks out the exact tag if present.
git_checkout_tag() {
    local url="$1"
    local dir="$2"
    local tag="$3"

    if [[ -d "$dir/.git" ]]; then
        log "Updating existing clone: $dir"
        git -C "$dir" fetch --tags --quiet
        git -C "$dir" checkout "$tag" --quiet
    else
        log "Cloning $url → $dir (tag: $tag)"
        mkdir -p "$(dirname "$dir")"
        git clone --depth 50 "$url" "$dir" --quiet
        git -C "$dir" fetch --tags --quiet
        git -C "$dir" checkout "$tag" --quiet
    fi
}

# ── Version store helpers ────────────────────────────────────────────────────

# read_installed_version <name>  — echoes stored version or ""
read_installed_version() {
    local name="$1"
    if [[ -f "${VERSION_DIR}/${name}" ]]; then
        cat "${VERSION_DIR}/${name}"
    else
        echo ""
    fi
}

# write_installed_version <name> <version>
write_installed_version() {
    local name="$1"
    local version="$2"
    mkdir -p "$VERSION_DIR"
    echo "$version" > "${VERSION_DIR}/${name}"
}

# ── File helpers ─────────────────────────────────────────────────────────────

# write_if_changed <path> <content>
# Writes content to path only if the current content differs.
# Returns 0 if written, 1 if skipped.
write_if_changed() {
    local path="$1"
    local content="$2"
    if [[ -f "$path" ]] && [[ "$(cat "$path")" == "$content" ]]; then
        skip "file unchanged: $path"
        return 1
    fi
    echo "$content" > "$path"
    log "wrote: $path"
    return 0
}

# ensure_line_in_file <line> <file>
# Idempotently adds a line to a file if not already present.
ensure_line_in_file() {
    local line="$1"
    local file="$2"
    if grep -qxF "$line" "$file" 2>/dev/null; then
        return 0
    fi
    echo "$line" >> "$file"
    log "added '$line' to $file"
}

# ── Apt helpers ──────────────────────────────────────────────────────────────

# apt_update_if_stale  — runs apt-get update only if cache is older than 1 hour
apt_update_if_stale() {
    local cache="/var/cache/apt/pkgcache.bin"
    local stale=1
    if [[ -f "$cache" ]]; then
        local age=$(( $(date +%s) - $(stat -c %Y "$cache") ))
        if (( age < 3600 )); then
            stale=0
        fi
    fi
    if (( stale )); then
        log "apt-get update (cache is stale)"
        apt-get update -qq
    else
        skip "apt cache is fresh (< 1 hour old)"
    fi
}

# ── Summary tracking ─────────────────────────────────────────────────────────

record_done() { DONE_ITEMS+=("$1"); }
record_skip() { SKIPPED_ITEMS+=("$1"); }

print_summary() {
    echo ""
    echo "════════════════════════════════════════════════"
    echo "  AmpliPi Setup Summary"
    echo "════════════════════════════════════════════════"
    if (( ${#DONE_ITEMS[@]} )); then
        echo -e "${_CLR_GREEN}Done:${_CLR_RESET}"
        for item in "${DONE_ITEMS[@]}"; do echo "  ✓ $item"; done
    fi
    if (( ${#SKIPPED_ITEMS[@]} )); then
        echo -e "${_CLR_YELLOW}Skipped (already done):${_CLR_RESET}"
        for item in "${SKIPPED_ITEMS[@]}"; do echo "  · $item"; done
    fi
    if (( REBOOT_NEEDED )); then
        echo ""
        echo -e "${_CLR_YELLOW}⚠  Boot configuration changed — REBOOT REQUIRED${_CLR_RESET}"
        echo "   Run: sudo reboot"
    fi
    echo "════════════════════════════════════════════════"
}
