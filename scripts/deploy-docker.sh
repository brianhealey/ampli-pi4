#!/bin/bash
# deploy-docker.sh - Automated deployment to Raspberry Pi
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

# Default values
PI_HOST="${PI_HOST:-pi@amplipi.local}"
COMPOSE_FILE="deployments/docker-compose.prod.yml"
ENV_FILE="deployments/.env.example"

# Detect current git branch for image tags
GIT_BRANCH=$(cd "$PROJECT_ROOT" && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "main")
IMAGE_TAG="${IMAGE_TAG:-$GIT_BRANCH}"

# Parse command line arguments
SKIP_MIGRATION=false
SKIP_PULL=false
DRY_RUN=false
FORCE_ENV=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-migration)
            SKIP_MIGRATION=true
            shift
            ;;
        --skip-pull)
            SKIP_PULL=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --force-env)
            FORCE_ENV=true
            shift
            ;;
        --host)
            PI_HOST="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --skip-migration     Skip the migration script"
            echo "  --skip-pull          Skip pulling Docker images"
            echo "  --dry-run            Show what would be done without executing"
            echo "  --force-env          Regenerate .env file even if it exists"
            echo "  --host <user@host>   Specify Pi host (default: pi@amplipi.local)"
            echo "  --help               Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Function to print colored messages
success() { echo -e "${GREEN}✓${NC} $1"; }
warning() { echo -e "${YELLOW}⚠${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; }
info() { echo -e "${BLUE}ℹ${NC} $1"; }

# Function to run command on Pi
run_on_pi() {
    if [ "$DRY_RUN" = true ]; then
        echo "  [DRY RUN] ssh $PI_HOST '$*'"
    else
        ssh -o ConnectTimeout=10 "$PI_HOST" "$@"
    fi
}

# Function to copy file to Pi
copy_to_pi() {
    local src=$1
    local dest=$2
    if [ "$DRY_RUN" = true ]; then
        echo "  [DRY RUN] scp $src $PI_HOST:$dest"
    else
        scp -o ConnectTimeout=10 "$src" "$PI_HOST:$dest"
    fi
}

# Function to generate .env file with current branch tags
generate_env_file() {
    local dest=$1
    cat > "$dest" << EOF
# AmpliPi Docker Environment - Auto-generated for branch: $IMAGE_TAG
# Generated: $(date -u +"%Y-%m-%d %H:%M:%S UTC")

# Container Images (using branch-specific tags)
AMPLIPI_IMAGE=ghcr.io/brianhealey/amplipi:$IMAGE_TAG
DISPLAY_IMAGE=ghcr.io/brianhealey/amplipi-display:$IMAGE_TAG
AIRPLAY_IMAGE=ghcr.io/brianhealey/amplipi-airplay:$IMAGE_TAG
SPOTIFY_IMAGE=ghcr.io/brianhealey/amplipi-spotify:$IMAGE_TAG
PANDORA_IMAGE=ghcr.io/brianhealey/amplipi-pandora:$IMAGE_TAG
LMS_IMAGE=ghcr.io/brianhealey/amplipi-lms:$IMAGE_TAG
DLNA_IMAGE=ghcr.io/brianhealey/amplipi-dlna:$IMAGE_TAG

# Logging
LOG_LEVEL=info

# Hardware Configuration
HARDWARE_MOCK=false
DISPLAY_TYPE=tft

# Macvlan Network Configuration
# IMPORTANT: Adjust these to match your network
MACVLAN_PARENT=eth0
MACVLAN_SUBNET=192.168.1.0/24
MACVLAN_GATEWAY=192.168.1.1
MACVLAN_IP_RANGE=192.168.1.100/30

# AirPlay2 IP Addresses
# IMPORTANT: Reserve these IPs outside your DHCP range
AIRPLAY1_IP=192.168.1.100
AIRPLAY2_IP=192.168.1.101
AIRPLAY3_IP=192.168.1.102
AIRPLAY4_IP=192.168.1.103

# Spotify Configuration (if using streaming profile)
SPOTIFY_BITRATE=320     # 96, 160, 320
EOF
}

echo -e "${BLUE}=== AmpliPi Docker Deployment ===${NC}"
echo ""
info "Target host: $PI_HOST"
info "Image tag: $IMAGE_TAG (from git branch: $GIT_BRANCH)"
info "Registry: ghcr.io/brianhealey"
echo ""

# 1. Check connectivity to Pi
info "Checking connectivity to Pi..."
if [ "$DRY_RUN" = false ]; then
    if ! ssh -o ConnectTimeout=10 -o BatchMode=yes "$PI_HOST" "echo 'Connected'" >/dev/null 2>&1; then
        error "Cannot connect to $PI_HOST"
        echo ""
        echo "Please ensure:"
        echo "  1. The Pi is powered on and connected to the network"
        echo "  2. SSH is enabled on the Pi"
        echo "  3. You can SSH without password (use ssh-copy-id if needed)"
        echo "  4. The hostname/IP is correct"
        exit 1
    fi
fi
success "Connected to Pi"

# 2. Check if Docker is installed on Pi
echo ""
info "Checking Docker installation on Pi..."
if [ "$DRY_RUN" = false ]; then
    if ! run_on_pi "command -v docker >/dev/null 2>&1"; then
        error "Docker is not installed on Pi"
        echo ""
        echo "Please install Docker first:"
        echo "  curl -fsSL https://get.docker.com | sh"
        echo "  sudo usermod -aG docker \$USER"
        exit 1
    fi
    if ! run_on_pi "command -v docker-compose >/dev/null 2>&1 || docker compose version >/dev/null 2>&1"; then
        error "Docker Compose is not installed on Pi"
        echo ""
        echo "Docker Compose v2 should be installed with Docker. If missing:"
        echo "  sudo apt-get update && sudo apt-get install docker-compose-plugin"
        exit 1
    fi
fi
success "Docker and Docker Compose are installed"

# 3. Copy deployment files to Pi
echo ""
info "Copying deployment files to Pi..."

# Create remote directory if it doesn't exist
run_on_pi "mkdir -p /home/pi/amplipi-docker"

# Copy docker-compose file
copy_to_pi "$PROJECT_ROOT/$COMPOSE_FILE" "/home/pi/amplipi-docker/docker-compose.prod.yml"
success "Copied docker-compose.prod.yml"

# Handle .env file
# Check if .env already exists on Pi
ENV_EXISTS=false
if [ "$DRY_RUN" = false ] && [ "$FORCE_ENV" = false ]; then
    if run_on_pi "test -f /home/pi/amplipi-docker/.env" 2>/dev/null; then
        ENV_EXISTS=true
    fi
fi

if [ "$ENV_EXISTS" = true ]; then
    warning ".env file already exists on Pi - keeping existing configuration"
    info "To update .env, use --force-env flag or manually edit/remove it on the Pi"
elif [ -f "$PROJECT_ROOT/deployments/.env" ]; then
    # Use existing .env from local deployments/
    copy_to_pi "$PROJECT_ROOT/deployments/.env" "/home/pi/amplipi-docker/.env"
    success "Copied .env file from deployments/"
else
    # Generate .env with branch-specific image tags
    TEMP_ENV=$(mktemp)
    generate_env_file "$TEMP_ENV"
    copy_to_pi "$TEMP_ENV" "/home/pi/amplipi-docker/.env"
    rm "$TEMP_ENV"
    success "Generated .env file with image tags for branch: $IMAGE_TAG"
    warning "Please review and customize network settings in .env on the Pi"
fi

# Copy migration script
copy_to_pi "$SCRIPT_DIR/migrate-to-docker.sh" "/home/pi/amplipi-docker/migrate-to-docker.sh"
run_on_pi "chmod +x /home/pi/amplipi-docker/migrate-to-docker.sh"
success "Copied migration script"

# 4. Run migration script (unless skipped)
if [ "$SKIP_MIGRATION" = false ]; then
    echo ""
    info "Running migration script..."
    run_on_pi "cd /home/pi/amplipi-docker && ./migrate-to-docker.sh"
    success "Migration complete"
else
    warning "Skipping migration (--skip-migration specified)"
fi

# 5. Pull Docker images (unless skipped)
if [ "$SKIP_PULL" = false ]; then
    echo ""
    info "Pulling Docker images (this may take several minutes)..."
    # Use sudo if user doesn't have Docker permissions
    run_on_pi "cd /home/pi/amplipi-docker && (docker info >/dev/null 2>&1 && docker compose -f docker-compose.prod.yml pull || sudo docker compose -f docker-compose.prod.yml pull)"
    success "Docker images pulled"
else
    warning "Skipping image pull (--skip-pull specified)"
fi

# 6. Start Docker Compose stack
echo ""
info "Starting Docker Compose stack..."
# Use sudo if user doesn't have Docker permissions
run_on_pi "cd /home/pi/amplipi-docker && (docker info >/dev/null 2>&1 && docker compose -f docker-compose.prod.yml up -d || sudo docker compose -f docker-compose.prod.yml up -d)"
success "Docker containers started"

# 7. Wait for services to be ready
echo ""
info "Waiting for services to start..."
if [ "$DRY_RUN" = false ]; then
    sleep 10
fi

# 8. Check container status
echo ""
info "Checking container status..."
if [ "$DRY_RUN" = false ]; then
    CONTAINER_STATUS=$(run_on_pi "docker ps --filter name=amplipi --format '{{.Names}}: {{.Status}}'")
    echo "$CONTAINER_STATUS"
fi

# 9. Test API endpoint
echo ""
info "Testing API endpoint..."
if [ "$DRY_RUN" = false ]; then
    if run_on_pi "curl -sf http://localhost/api/info >/dev/null 2>&1"; then
        success "API is responding"
    else
        warning "API not responding yet - may still be starting up"
        info "Check logs with: ssh $PI_HOST 'docker logs -f amplipi'"
    fi
fi

echo ""
echo -e "${GREEN}=== Deployment Complete ===${NC}"
echo ""
echo "Next steps:"
echo "  1. Access web interface: http://amplipi.local"
echo "  2. Check logs: ssh $PI_HOST 'docker logs -f amplipi'"
echo "  3. Check container status: ssh $PI_HOST 'docker ps'"
echo "  4. Test AirPlay discovery from iOS/macOS device"
echo ""

if [ "$DRY_RUN" = true ]; then
    warning "This was a DRY RUN - no changes were made"
fi
