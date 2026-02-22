#!/usr/bin/env bash
# deploy.sh — Deploy pre-built AmpliPi binary to the device
# Usage: ./deploy.sh /path/to/amplipi-binary [--restart|--start|--stop]

set -euo pipefail

# Deployment configuration
INSTALL_DIR="/home/pi/amplipi-go"
BINARY_NAME="amplipi"
SYSTEMD_SERVICE="amplipi.service"

# Check for binary argument
if [[ $# -lt 1 ]]; then
    echo "Usage: $0 /path/to/amplipi-binary [--restart|--start|--stop]"
    exit 1
fi

BINARY_SRC="$1"
ACTION="${2:-restart}"

if [[ ! -f "$BINARY_SRC" ]]; then
    echo "Error: Binary not found: $BINARY_SRC"
    exit 1
fi

echo "==> Deploying AmpliPi binary"

# Stop service
echo "Stopping $SYSTEMD_SERVICE..."
sudo systemctl stop "$SYSTEMD_SERVICE" || true

# Create installation directory
mkdir -p "$INSTALL_DIR"

# Copy binary
echo "Installing binary to $INSTALL_DIR/$BINARY_NAME..."
cp "$BINARY_SRC" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Update systemd service if scripts/configs/amplipi.service exists
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_CONFIG="$SCRIPT_DIR/configs/$SYSTEMD_SERVICE"

if [[ -f "$SERVICE_CONFIG" ]]; then
    echo "Updating systemd service configuration..."
    sudo cp "$SERVICE_CONFIG" "/etc/systemd/system/$SYSTEMD_SERVICE"
    sudo systemctl daemon-reload
else
    echo "Warning: Service config not found at $SERVICE_CONFIG"
fi

# Handle service action
case "$ACTION" in
    --start|start)
        echo "Starting $SYSTEMD_SERVICE..."
        sudo systemctl start "$SYSTEMD_SERVICE"
        sudo systemctl status "$SYSTEMD_SERVICE" --no-pager -l
        ;;
    --restart|restart)
        echo "Restarting $SYSTEMD_SERVICE..."
        sudo systemctl restart "$SYSTEMD_SERVICE"
        sudo systemctl status "$SYSTEMD_SERVICE" --no-pager -l
        ;;
    --stop|stop)
        echo "Service stopped (not restarting)"
        ;;
    *)
        echo "Unknown action: $ACTION"
        exit 1
        ;;
esac

echo "Deployment complete"
