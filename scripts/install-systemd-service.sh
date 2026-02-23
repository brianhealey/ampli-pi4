#!/bin/bash
# install-systemd-service.sh - Install and enable systemd service for Docker Compose stack
# Part of Docker Container Migration feature

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored messages
success() { echo -e "${GREEN}✓${NC} $1"; }
warning() { echo -e "${YELLOW}⚠${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; }
info() { echo -e "${BLUE}ℹ${NC} $1"; }

echo -e "${BLUE}=== AmpliPi Systemd Service Installation ===${NC}"
echo ""

# Check if running as root or with sudo
if [ "$EUID" -ne 0 ]; then
    error "This script must be run with sudo"
    echo "Usage: sudo $0"
    exit 1
fi

# Check if running on Raspberry Pi
if [ ! -f /proc/device-tree/model ] || ! grep -q "Raspberry Pi" /proc/device-tree/model 2>/dev/null; then
    warning "Not running on Raspberry Pi - proceeding anyway"
fi

# 1. Check if Docker is installed
info "Checking Docker installation..."
if ! command -v docker >/dev/null 2>&1; then
    error "Docker is not installed"
    echo "Please install Docker first:"
    echo "  curl -fsSL https://get.docker.com | sh"
    exit 1
fi
if ! command -v docker-compose >/dev/null 2>&1; then
    error "Docker Compose is not installed"
    echo "Please install Docker Compose v2:"
    echo "  sudo apt-get update && sudo apt-get install docker-compose-plugin"
    exit 1
fi
success "Docker and Docker Compose are installed"

# 2. Check if deployment directory exists
DEPLOY_DIR="/home/pi/amplipi-docker"
if [ ! -d "$DEPLOY_DIR" ]; then
    warning "Deployment directory $DEPLOY_DIR does not exist"
    info "Creating deployment directory..."
    mkdir -p "$DEPLOY_DIR"
    success "Created $DEPLOY_DIR"
fi

# 3. Check if docker-compose.prod.yml exists
if [ ! -f "$DEPLOY_DIR/docker-compose.prod.yml" ]; then
    warning "docker-compose.prod.yml not found in $DEPLOY_DIR"
    if [ -f "$PROJECT_ROOT/deployments/docker-compose.prod.yml" ]; then
        info "Copying docker-compose.prod.yml from project..."
        cp "$PROJECT_ROOT/deployments/docker-compose.prod.yml" "$DEPLOY_DIR/"
        success "Copied docker-compose.prod.yml"
    else
        error "docker-compose.prod.yml not found"
        echo "Please run the deployment script first:"
        echo "  ./scripts/deploy-docker.sh"
        exit 1
    fi
fi

# 4. Stop and disable old services if they exist
info "Checking for old services..."
OLD_SERVICES=("amplipi" "amplipi-display")
for service in "${OLD_SERVICES[@]}"; do
    if systemctl is-active --quiet "$service" 2>/dev/null; then
        info "Stopping old service: $service"
        systemctl stop "$service"
        success "Stopped $service"
    fi
    if systemctl is-enabled --quiet "$service" 2>/dev/null; then
        info "Disabling old service: $service"
        systemctl disable "$service"
        success "Disabled $service"
    fi
done

# 5. Copy systemd service file
SERVICE_FILE="amplipi-docker.service"
SOURCE_SERVICE="$PROJECT_ROOT/deployments/$SERVICE_FILE"
DEST_SERVICE="/etc/systemd/system/$SERVICE_FILE"

if [ ! -f "$SOURCE_SERVICE" ]; then
    error "Systemd service file not found: $SOURCE_SERVICE"
    exit 1
fi

info "Installing systemd service..."
cp "$SOURCE_SERVICE" "$DEST_SERVICE"
chmod 644 "$DEST_SERVICE"
success "Copied $SERVICE_FILE to /etc/systemd/system/"

# 6. Reload systemd daemon
info "Reloading systemd daemon..."
systemctl daemon-reload
success "Systemd daemon reloaded"

# 7. Enable service to start on boot
info "Enabling service to start on boot..."
systemctl enable "$SERVICE_FILE"
success "Service enabled"

# 8. Start the service
info "Starting service..."
systemctl start "$SERVICE_FILE"

# Wait a moment for service to start
sleep 3

# 9. Check service status
if systemctl is-active --quiet "$SERVICE_FILE"; then
    success "Service is running"
else
    warning "Service failed to start"
    error "Check status with: sudo systemctl status $SERVICE_FILE"
    exit 1
fi

# 10. Display service status
echo ""
info "Service status:"
systemctl status "$SERVICE_FILE" --no-pager -l || true

echo ""
echo -e "${GREEN}=== Installation Complete ===${NC}"
echo ""
echo "Service commands:"
echo "  Status:  sudo systemctl status $SERVICE_FILE"
echo "  Start:   sudo systemctl start $SERVICE_FILE"
echo "  Stop:    sudo systemctl stop $SERVICE_FILE"
echo "  Restart: sudo systemctl restart $SERVICE_FILE"
echo "  Logs:    sudo journalctl -u $SERVICE_FILE -f"
echo ""
echo "The AmpliPi Docker stack will now start automatically on boot."
echo ""
