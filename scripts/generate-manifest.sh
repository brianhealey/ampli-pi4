#!/bin/bash
# generate-manifest.sh - Generate release manifest for Docker deployment
# Usage: ./generate-manifest.sh <version> <registry> <owner>

set -e

VERSION="${1:-}"
REGISTRY="${2:-ghcr.io}"
OWNER="${3:-brianhealey}"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version> [registry] [owner]"
  echo "Example: $0 v1.0.0 ghcr.io brianhealey"
  exit 1
fi

# Remove 'v' prefix if present
VERSION_NUM="${VERSION#v}"

# Get current timestamp in ISO 8601 format
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Images to include in manifest
IMAGES=(
  "amplipi"
  "amplipi-display"
  "amplipi-airplay"
  "amplipi-spotify"
  "amplipi-pandora"
  "amplipi-lms"
  "amplipi-dlna"
)

echo "Generating manifest for version: $VERSION_NUM"
echo "Registry: $REGISTRY"
echo "Owner: $OWNER"
echo ""

# Start manifest file
cat > manifest.yml <<EOF
version: "$VERSION_NUM"
release_date: "$TIMESTAMP"
description: |
  AmpliPi Docker Container Release $VERSION_NUM

  This release includes containerized versions of all AmpliPi services:
  - Main application with web interface and API
  - Display driver for TFT/eInk screens
  - Four AirPlay2 instances with macvlan networking
  - Optional streaming services (Spotify, Pandora, LMS, DLNA)

images:
EOF

# Add each image to manifest
for IMAGE in "${IMAGES[@]}"; do
  IMAGE_REF="${REGISTRY}/${OWNER}/${IMAGE}:${VERSION}"

  echo "  Fetching digest for ${IMAGE}..."

  # Try to get digest from Docker
  if command -v docker &> /dev/null; then
    # Try to inspect local image first
    DIGEST=$(docker inspect "${IMAGE_REF}" --format='{{index .RepoDigests 0}}' 2>/dev/null | cut -d'@' -f2 || echo "")

    # If not found locally, try to pull and get digest
    if [ -z "$DIGEST" ]; then
      echo "    (Image not found locally, pulling...)"
      docker pull "${IMAGE_REF}" >/dev/null 2>&1 || echo "    Warning: Could not pull image"
      DIGEST=$(docker inspect "${IMAGE_REF}" --format='{{index .RepoDigests 0}}' 2>/dev/null | cut -d'@' -f2 || echo "sha256:0000000000000000000000000000000000000000000000000000000000000000")
    fi
  else
    # Docker not available, use placeholder
    echo "    Warning: Docker not available, using placeholder digest"
    DIGEST="sha256:0000000000000000000000000000000000000000000000000000000000000000"
  fi

  # Add image to manifest
  cat >> manifest.yml <<EOF
  ${IMAGE}:
    registry: ${REGISTRY}
    repository: ${OWNER}/${IMAGE}
    tag: ${VERSION}
    digest: ${DIGEST}
    platforms:
      - linux/arm64
      - linux/amd64
EOF

  # Add size estimate (optional)
  if command -v docker &> /dev/null && [ -n "$(docker images -q ${IMAGE_REF})" ]; then
    SIZE_MB=$(docker image inspect "${IMAGE_REF}" --format='{{.Size}}' 2>/dev/null | awk '{print int($1/1024/1024)}' || echo "0")
    if [ "$SIZE_MB" -gt 0 ]; then
      echo "    size_mb: ${SIZE_MB}" >> manifest.yml
    fi
  fi

  echo ""
done

# Add deployment configuration
cat >> manifest.yml <<EOF

deployment:
  compose_file: docker-compose.prod.yml
  env_template: .env.example

requirements:
  docker_engine: ">=24.0"
  docker_compose: ">=2.0"
  os: linux
  arch:
    - arm64
    - amd64
  min_disk_space_gb: 5
  min_memory_mb: 512

changelog:
  - Docker containerization of all AmpliPi services
  - Multi-architecture support (ARM64 + AMD64)
  - Macvlan networking for four independent AirPlay2 instances
  - Automated deployment and migration scripts
  - Systemd service integration for automatic startup
EOF

# Calculate checksum of docker-compose.prod.yml if it exists
if [ -f "deployments/docker-compose.prod.yml" ]; then
  COMPOSE_CHECKSUM=$(sha256sum deployments/docker-compose.prod.yml | awk '{print $1}')
  sed -i.bak "s|compose_file: docker-compose.prod.yml|compose_file: docker-compose.prod.yml\n  compose_checksum: sha256:${COMPOSE_CHECKSUM}|" manifest.yml
  rm manifest.yml.bak
fi

# Calculate checksum of .env.example if it exists
if [ -f "deployments/.env.example" ]; then
  ENV_CHECKSUM=$(sha256sum deployments/.env.example | awk '{print $1}')
  sed -i.bak "s|env_template: .env.example|env_template: .env.example\n  env_checksum: sha256:${ENV_CHECKSUM}|" manifest.yml
  rm manifest.yml.bak
fi

echo "✓ Manifest generated: manifest.yml"
echo ""
cat manifest.yml
