#!/usr/bin/env bash
# 10-system.sh — System prerequisites
# Sourced by setup.sh. Requires common.sh to be sourced first.

set -euo pipefail

step "10 · System prerequisites"

# ── Clear any invalid inherited LC_* variables ────────────────────────────────
# A previous partial locale setup may have left LC_MESSAGES/LC_ALL pointing at
# a locale that isn't generated (e.g. en_US.UTF-8 on an en_GB system).
# Unset them so apt/perl don't spew warnings throughout the run.
for _lc_var in LC_ALL LC_MESSAGES LC_CTYPE LC_NUMERIC LC_TIME LC_COLLATE \
               LC_MONETARY LC_PAPER LC_ADDRESS LC_TELEPHONE LC_NAME \
               LC_MEASUREMENT LC_IDENTIFICATION LANGUAGE; do
    _lc_val="${!_lc_var:-}"
    if [[ -n "$_lc_val" ]]; then
        if ! locale -a 2>/dev/null | grep -qxF "${_lc_val%.*}.UTF-8" && \
           ! locale -a 2>/dev/null | grep -qxF "$_lc_val"; then
            unset "$_lc_var"
            log "Cleared invalid locale var: $_lc_var=$_lc_val"
        fi
    fi
done

# ── Locale ───────────────────────────────────────────────────────────────────
# AmpliPi doesn't require a specific locale — just ensure a UTF-8 locale is
# configured. Accept any *.UTF-8 LANG; don't fight the OS default.
_current_lang="$(locale 2>/dev/null | grep '^LANG=' | cut -d= -f2 | tr -d '"')"
if [[ "$_current_lang" == *"UTF-8"* ]] || [[ "$_current_lang" == *"utf8"* ]]; then
    skip "locale (${_current_lang} — UTF-8 already configured)"
    record_skip "locale"
else
    step "Generating and setting en_US.UTF-8 locale"
    if command -v locale-gen &>/dev/null; then
        locale-gen en_US.UTF-8 2>/dev/null || warn "locale-gen failed — continuing"
    fi
    update-locale LANG=en_US.UTF-8 2>/dev/null || warn "update-locale failed — continuing"
    log "locale configured (reboot may be needed to fully apply)"
    record_done "locale"
fi

# ── Timezone ─────────────────────────────────────────────────────────────────
_desired_tz="UTC"

if [[ "$(cat /etc/timezone 2>/dev/null)" == "$_desired_tz" ]]; then
    skip "timezone (${_desired_tz})"
    record_skip "timezone"
else
    step "Setting timezone to ${_desired_tz}"
    ln -sf "/usr/share/zoneinfo/${_desired_tz}" /etc/localtime
    echo "$_desired_tz" > /etc/timezone
    dpkg-reconfigure -f noninteractive tzdata 2>/dev/null || true
    log "timezone set to ${_desired_tz}"
    record_done "timezone"
fi

# ── Persistent journal ───────────────────────────────────────────────────────
# journald defaults to Storage=auto, which only persists if /var/log/journal
# exists — it does not by default, so every reboot discards the log. That is
# painful on a unit whose interesting failures are hardware-adjacent and often
# only visible across a boot: which SPI controller bound, whether a USB device
# ever enumerated, what the fan and temperature registers were doing.
#
# Capped deliberately: this runs on an SD card, and an uncapped journal is both
# a space and a write-endurance problem.
step "Persistent journal"

_journal_conf_dir="/etc/systemd/journald.conf.d"
_journal_conf="$_journal_conf_dir/amplipi.conf"
_journal_content="# Installed by AmpliPi setup.sh — keep logs across reboots, but bounded
# so the journal cannot fill the SD card or wear it out.
[Journal]
Storage=persistent
SystemMaxUse=200M
SystemMaxFileSize=20M
"

mkdir -p "$_journal_conf_dir"
_journal_changed=0
if write_if_changed "$_journal_conf" "$_journal_content"; then
    _journal_changed=1
fi

# Raspberry Pi OS ships /usr/lib/systemd/journald.conf.d/40-rpi-volatile-storage.conf
# to spare the SD card. "amplipi.conf" sorts after "40-..." so this drop-in wins,
# but the directory must also exist before journald will use it.
if [[ ! -d /var/log/journal ]]; then
    mkdir -p /var/log/journal
    log "created /var/log/journal"
    _journal_changed=1
else
    skip "/var/log/journal (already exists)"
fi
# Run unconditionally: the directory may exist with the wrong ownership or ACLs,
# in which case journald silently keeps logging to tmpfs.
systemd-tmpfiles --create --prefix /var/log/journal &>/dev/null || true

if (( _journal_changed )); then
    systemctl restart systemd-journald 2>/dev/null || warn "could not restart systemd-journald"
    # Restarting alone is not enough — the running journal stays in /run until
    # it is flushed across, which otherwise only happens at the next boot.
    journalctl --flush &>/dev/null || true
    log "journald now persists logs across reboots (capped at 200M)"
    record_done "persistent journal"
else
    record_skip "persistent journal"
fi

# ── Passwordless sudo for pi user (amplipi systemd commands) ─────────────────
_sudoers_file="/etc/sudoers.d/amplipi"
_sudoers_content="# AmpliPi: allow pi user to manage amplipi systemd services without password
pi ALL=(ALL) NOPASSWD: /bin/systemctl start amplipi
pi ALL=(ALL) NOPASSWD: /bin/systemctl stop amplipi
pi ALL=(ALL) NOPASSWD: /bin/systemctl restart amplipi
pi ALL=(ALL) NOPASSWD: /bin/systemctl status amplipi
pi ALL=(ALL) NOPASSWD: /bin/systemctl start amplipi-update
pi ALL=(ALL) NOPASSWD: /bin/systemctl stop amplipi-update
pi ALL=(ALL) NOPASSWD: /bin/systemctl restart amplipi-update
pi ALL=(ALL) NOPASSWD: /bin/systemctl status amplipi-update"

if [[ -f "$_sudoers_file" ]] && [[ "$(cat "$_sudoers_file")" == "$_sudoers_content" ]]; then
    skip "sudoers (${_sudoers_file})"
    record_skip "sudoers"
else
    step "Writing ${_sudoers_file}"
    echo "$_sudoers_content" > "$_sudoers_file"
    chmod 0440 "$_sudoers_file"
    # Validate
    if visudo -c -f "$_sudoers_file" &>/dev/null; then
        log "sudoers file written and validated"
        record_done "sudoers"
    else
        error "sudoers file failed validation — removing"
        rm -f "$_sudoers_file"
        exit 1
    fi
fi

# ── apt-get update (if stale) ────────────────────────────────────────────────
step "apt update"
apt_update_if_stale

# ── Base packages ────────────────────────────────────────────────────────────
step "Base packages"
_base_pkgs=(
    git
    curl
    wget
    build-essential
    pkg-config
    autoconf
    automake
    libtool
    cmake
    python3-pip
)

apt-get install -y --no-install-recommends "${_base_pkgs[@]}"
log "base packages installed"
record_done "base packages"
