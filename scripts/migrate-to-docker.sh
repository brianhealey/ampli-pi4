#!/bin/bash
# migrate-to-docker.sh - Migrate existing config to Docker volumes
# Part of Docker Container Migration feature

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== AmpliPi Docker Migration ===${NC}"
echo ""

# Function to print success messages
success() {
    echo -e "${GREEN}✓${NC} $1"
}

# Function to print warning messages
warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# Function to print error messages
error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if running on Raspberry Pi
if [ ! -f /proc/device-tree/model ] || ! grep -q "Raspberry Pi" /proc/device-tree/model 2>/dev/null; then
    warning "Not running on Raspberry Pi - proceeding anyway for testing"
fi

# 1. Check if existing configuration exists
CONFIG_SOURCE="/home/pi/.config/amplipi"
if [ -d "$CONFIG_SOURCE" ]; then
    success "Found existing configuration at $CONFIG_SOURCE"
else
    warning "No existing configuration found at $CONFIG_SOURCE"
    warning "Fresh installation - volumes will be empty"
fi

# 2. Stop existing services if they exist
echo ""
echo "Stopping existing AmpliPi services..."
if systemctl is-active --quiet amplipi 2>/dev/null; then
    sudo systemctl stop amplipi
    success "Stopped amplipi service"
else
    warning "amplipi service not running or doesn't exist"
fi

if systemctl is-active --quiet amplipi-display 2>/dev/null; then
    sudo systemctl stop amplipi-display
    success "Stopped amplipi-display service"
else
    warning "amplipi-display service not running or doesn't exist"
fi

# 3. Create Docker volumes if they don't exist
echo ""
echo "Creating Docker volumes..."

# Check if we have Docker permissions
if ! docker info >/dev/null 2>&1; then
    warning "No Docker permissions - trying with sudo"
    DOCKER_CMD="sudo docker"
else
    DOCKER_CMD="docker"
fi

if $DOCKER_CMD volume inspect amplipi-config >/dev/null 2>&1; then
    warning "Volume amplipi-config already exists - skipping creation"
else
    $DOCKER_CMD volume create amplipi-config
    success "Created volume: amplipi-config"
fi

if $DOCKER_CMD volume inspect amplipi-logs >/dev/null 2>&1; then
    warning "Volume amplipi-logs already exists - skipping creation"
else
    $DOCKER_CMD volume create amplipi-logs
    success "Created volume: amplipi-logs"
fi

# 4. Copy existing configuration to Docker volume (if exists)
if [ -d "$CONFIG_SOURCE" ]; then
    echo ""
    echo "Migrating configuration to Docker volume..."

    # Use a temporary container to copy files into the volume
    $DOCKER_CMD run --rm \
        -v amplipi-config:/dest \
        -v "$CONFIG_SOURCE:/source:ro" \
        alpine sh -c "cp -av /source/. /dest/"

    success "Configuration migrated to amplipi-config volume"
else
    warning "No configuration to migrate"
fi

# 5. Copy existing logs to Docker volume (if exists)
LOG_SOURCE="/var/log/amplipi"
if [ -d "$LOG_SOURCE" ]; then
    echo ""
    echo "Migrating logs to Docker volume..."

    $DOCKER_CMD run --rm \
        -v amplipi-logs:/dest \
        -v "$LOG_SOURCE:/source:ro" \
        alpine sh -c "cp -av /source/. /dest/"

    success "Logs migrated to amplipi-logs volume"
else
    warning "No logs to migrate"
fi

# 6. Configure avahi-daemon to only advertise on physical network interface
echo ""
echo "Configuring avahi-daemon for Docker compatibility..."
AVAHI_CONF="/etc/avahi/avahi-daemon.conf"
if [ -f "$AVAHI_CONF" ]; then
    # Uncomment allow-interfaces=eth0 to prevent advertising on Docker bridge interfaces
    if grep -q "^#allow-interfaces=eth0" "$AVAHI_CONF"; then
        sudo sed -i 's/^#allow-interfaces=eth0/allow-interfaces=eth0/' "$AVAHI_CONF"
        success "Configured avahi to only advertise on eth0"
        # Restart avahi to apply changes
        sudo systemctl restart avahi-daemon
        success "Restarted avahi-daemon"
    elif grep -q "^allow-interfaces=eth0" "$AVAHI_CONF"; then
        success "Avahi already configured for eth0 only"
    else
        warning "Could not find allow-interfaces line in avahi-daemon.conf"
    fi
else
    warning "Avahi daemon configuration not found"
fi

# 7. Create backup of original files (optional)
if [ -d "$CONFIG_SOURCE" ]; then
    echo ""
    BACKUP_DIR="/home/pi/amplipi-backup-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$BACKUP_DIR"
    cp -a "$CONFIG_SOURCE" "$BACKUP_DIR/config"
    if [ -d "$LOG_SOURCE" ]; then
        cp -a "$LOG_SOURCE" "$BACKUP_DIR/logs"
    fi
    success "Backup created at $BACKUP_DIR"
fi

# 8. Verify migration
echo ""
echo "Verifying migration..."

# Check volume contents
CONFIG_FILES=$($DOCKER_CMD run --rm -v amplipi-config:/check alpine sh -c "ls -la /check | wc -l")
if [ "$CONFIG_FILES" -gt 2 ]; then  # More than just . and ..
    success "Configuration volume contains files"
else
    warning "Configuration volume is empty"
fi

echo ""
echo -e "${GREEN}=== Migration Complete ===${NC}"
echo ""
echo "Next steps:"
echo "  1. Start Docker containers: docker-compose -f deployments/docker-compose.prod.yml up -d"
echo "  2. Verify containers are running: docker ps"
echo "  3. Check logs: docker logs -f amplipi"
echo "  4. Access web interface: http://amplipi.local"
echo ""
