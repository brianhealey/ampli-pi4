#!/bin/bash
# 05-docker.sh - Install Docker Engine and Docker Compose
# Part of Docker Container Migration setup

set -euo pipefail

step "05 · Installing Docker Engine"

# Check if Docker is already installed
if command -v docker >/dev/null 2>&1; then
    skip "Docker (already installed: $(docker --version | head -1))"
    record_skip "docker"
else
    log "Installing Docker Engine..."

    # Install using Docker's official install script
    if ! curl -fsSL https://get.docker.com | sh; then
        error "Failed to install Docker"
        exit 1
    fi

    log "Docker Engine installed: $(docker --version | head -1)"

    # Add pi user to docker group
    log "Adding user 'pi' to docker group..."
    if ! usermod -aG docker pi; then
        warn "Failed to add pi user to docker group"
    else
        log "User 'pi' added to docker group"
    fi

    record_done "docker"
fi

# Check if Docker Compose is installed
if command -v docker-compose >/dev/null 2>&1 || docker compose version >/dev/null 2>&1; then
    skip "Docker Compose (already installed)"
else
    log "Installing Docker Compose plugin..."
    apt-get update -qq
    apt-get install -y -qq docker-compose-plugin
    log "Docker Compose plugin installed"
fi

# Enable Docker service
log "Enabling Docker service..."
systemctl enable docker >/dev/null 2>&1 || true
systemctl start docker >/dev/null 2>&1 || true

# Wait for Docker to be ready
for i in {1..30}; do
    if docker info >/dev/null 2>&1; then
        log "Docker daemon is ready"
        break
    fi
    sleep 1
done

if ! docker info >/dev/null 2>&1; then
    warn "Docker daemon may not be fully ready yet"
fi
