#!/usr/bin/env bash
# configure-avahi.sh - Configure avahi-daemon for consistent amplipi.local hostname
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/configs/avahi-daemon.conf"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Configuring Avahi mDNS ===${NC}"

# Check if running on Pi
if [ ! -f /etc/avahi/avahi-daemon.conf ]; then
    echo -e "${RED}Error: /etc/avahi/avahi-daemon.conf not found${NC}"
    echo "This script should be run on the Raspberry Pi"
    exit 1
fi

# Check if config file exists
if [ ! -f "${CONFIG_FILE}" ]; then
    echo -e "${RED}Error: ${CONFIG_FILE} not found${NC}"
    exit 1
fi

# Backup existing config
echo "Backing up existing avahi-daemon.conf..."
sudo cp /etc/avahi/avahi-daemon.conf /etc/avahi/avahi-daemon.conf.backup.$(date +%Y%m%d-%H%M%S)

# Copy new config
echo "Installing new avahi-daemon.conf..."
sudo cp "${CONFIG_FILE}" /etc/avahi/avahi-daemon.conf

# Restart avahi-daemon
echo "Restarting avahi-daemon..."
sudo systemctl restart avahi-daemon

# Wait for service to start
sleep 2

# Check status
if systemctl is-active --quiet avahi-daemon; then
    HOSTNAME=$(systemctl status avahi-daemon | grep "avahi-daemon: running" | sed 's/.*running \[\(.*\)\].*/\1/')
    echo -e "${GREEN}✓ Avahi daemon is running${NC}"
    echo -e "${GREEN}✓ Hostname: ${HOSTNAME}${NC}"

    if [ "$HOSTNAME" = "amplipi.local" ]; then
        echo -e "${GREEN}✓ Hostname is correctly set to amplipi.local${NC}"
    else
        echo -e "${YELLOW}⚠ Hostname is ${HOSTNAME} (expected amplipi.local)${NC}"
        echo "This may resolve after a few seconds"
    fi
else
    echo -e "${RED}✗ Avahi daemon failed to start${NC}"
    exit 1
fi

echo -e "${GREEN}=== Configuration Complete ===${NC}"
echo "The Pi will now always advertise as amplipi.local"
