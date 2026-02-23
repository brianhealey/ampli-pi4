#!/bin/bash
# verify-docker.sh - Automated verification of Docker foundational infrastructure
# Run from amplipi-go/ directory

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0

echo "════════════════════════════════════════════════════════════════"
echo "  Docker Infrastructure Verification (Phases 1-2)"
echo "════════════════════════════════════════════════════════════════"
echo ""

# Check function
check() {
    local name="$1"
    local command="$2"

    echo -n "Checking: $name... "

    if eval "$command" > /dev/null 2>&1; then
        echo -e "${GREEN}✅ PASS${NC}"
        ((PASSED++))
        return 0
    else
        echo -e "${RED}❌ FAIL${NC}"
        ((FAILED++))
        return 1
    fi
}

# ============================================================================
# Phase 1: File Structure
# ============================================================================
echo "📁 Phase 1: File Structure"
echo "────────────────────────────────────────────────────────────────"

check "Docker directory exists" "test -d docker"
check "Deployments directory exists" "test -d deployments"
check "Releases directory exists" "test -d releases"
check ".dockerignore file exists" "test -f .dockerignore"
check ".env.example exists" "test -f deployments/.env.example"
check "Main Dockerfile exists" "test -f docker/Dockerfile.amplipi"
check "Display Dockerfile exists" "test -f docker/Dockerfile.display"
check "AirPlay Dockerfile exists" "test -f docker/Dockerfile.airplay"
check "Spotify Dockerfile exists" "test -f docker/Dockerfile.spotify"
check "Pandora Dockerfile exists" "test -f docker/Dockerfile.pandora"
check "LMS Dockerfile exists" "test -f docker/Dockerfile.lms"
check "DLNA Dockerfile exists" "test -f docker/Dockerfile.dlna"
check "Dev compose file exists" "test -f docker/docker-compose.yml"
check "Prod compose file exists" "test -f deployments/docker-compose.prod.yml"
check "Macvlan compose overlay exists" "test -f deployments/docker-compose.macvlan.yml"

echo ""

# ============================================================================
# Phase 2: Docker Validation
# ============================================================================
echo "🐳 Phase 2: Docker Validation"
echo "────────────────────────────────────────────────────────────────"

check "Docker is running" "docker info"
check "Docker Compose available" "docker compose version"
check "Docker Buildx available" "docker buildx version"

echo ""

# ============================================================================
# Phase 3: Compose File Validation
# ============================================================================
echo "📋 Phase 3: Compose File Validation"
echo "────────────────────────────────────────────────────────────────"

check "Dev compose syntax valid" "docker compose -f docker/docker-compose.yml config"
check "Prod compose syntax valid" "docker compose -f deployments/docker-compose.prod.yml config"
check "Macvlan overlay syntax valid" "docker compose -f deployments/docker-compose.prod.yml -f deployments/docker-compose.macvlan.yml config"

echo ""

# ============================================================================
# Phase 4: Environment Variables
# ============================================================================
echo "🔧 Phase 4: Environment Variables"
echo "────────────────────────────────────────────────────────────────"

check "Image variables defined" "grep -q 'AMPLIPI_IMAGE' deployments/.env.example"
check "Network variables defined" "grep -q 'MACVLAN_PARENT' deployments/.env.example"
check "AirPlay IPs defined" "grep -q 'AIRPLAY1_IP' deployments/.env.example"
check "Log level defined" "grep -q 'LOG_LEVEL' deployments/.env.example"

echo ""

# ============================================================================
# Phase 5: Makefile Targets
# ============================================================================
echo "🔨 Phase 5: Makefile Targets"
echo "────────────────────────────────────────────────────────────────"

check "docker-build target exists" "grep -q '^docker-build:' Makefile"
check "docker-push target exists" "grep -q '^docker-push:' Makefile"
check "docker-deploy target exists" "grep -q '^docker-deploy:' Makefile"
check "docker-stop target exists" "grep -q '^docker-stop:' Makefile"
check "docker-logs target exists" "grep -q '^docker-logs:' Makefile"

echo ""

# ============================================================================
# Phase 6: Docker Build Test (Optional - takes time)
# ============================================================================
echo "🏗️  Phase 6: Docker Build Test"
echo "────────────────────────────────────────────────────────────────"
echo -e "${YELLOW}Note: This will build the main image (~2-5 minutes)${NC}"
echo -n "Run build test? (y/N): "
read -r response

if [[ "$response" =~ ^[Yy]$ ]]; then
    echo "Building main application image..."
    if docker build -f docker/Dockerfile.amplipi -t amplipi:verify-test . > /tmp/docker-build.log 2>&1; then
        echo -e "${GREEN}✅ PASS${NC} - Main image built successfully"
        ((PASSED++))

        # Check image size
        SIZE=$(docker inspect amplipi:verify-test --format='{{.Size}}' | numfmt --to=iec 2>/dev/null || echo "unknown")
        echo "   Image size: $SIZE"
    else
        echo -e "${RED}❌ FAIL${NC} - Build failed (see /tmp/docker-build.log)"
        ((FAILED++))
    fi
else
    echo "Skipping build test"
fi

echo ""

# ============================================================================
# Phase 7: Dev Environment Test (Optional)
# ============================================================================
echo "🚀 Phase 7: Dev Environment Test"
echo "────────────────────────────────────────────────────────────────"
echo -e "${YELLOW}Note: This will start containers and test API${NC}"
echo -n "Run dev environment test? (y/N): "
read -r response

if [[ "$response" =~ ^[Yy]$ ]]; then
    echo "Starting development environment..."

    cd docker
    if docker compose up -d > /tmp/docker-compose.log 2>&1; then
        echo -e "${GREEN}✅ PASS${NC} - Containers started"
        ((PASSED++))

        # Wait for health check
        echo "Waiting for health check (30s)..."
        sleep 30

        # Check API
        if curl -sf http://localhost/api/info > /dev/null 2>&1; then
            echo -e "${GREEN}✅ PASS${NC} - API responding"
            ((PASSED++))
        else
            echo -e "${RED}❌ FAIL${NC} - API not responding"
            ((FAILED++))
        fi

        # Show container status
        echo ""
        echo "Container status:"
        docker compose ps

        # Cleanup
        echo ""
        echo "Cleaning up..."
        docker compose down > /dev/null 2>&1
        echo "Cleanup complete"
    else
        echo -e "${RED}❌ FAIL${NC} - Failed to start (see /tmp/docker-compose.log)"
        ((FAILED++))
    fi

    cd ..
else
    echo "Skipping dev environment test"
fi

echo ""

# ============================================================================
# Summary
# ============================================================================
echo "════════════════════════════════════════════════════════════════"
echo "  Verification Summary"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo -e "${GREEN}Passed: $PASSED${NC}"
echo -e "${RED}Failed: $FAILED${NC}"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}🎉 All checks passed! Foundational phase is verified.${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Test on Raspberry Pi: make docker-deploy"
    echo "  2. Implement User Story 1: /speckit.implement --phase 3"
    echo "  3. Build all images: make docker-build"
    exit 0
else
    echo -e "${RED}❌ Some checks failed. Review the output above.${NC}"
    echo ""
    echo "Common fixes:"
    echo "  - Ensure Docker is running: docker info"
    echo "  - Check Go version in Dockerfiles matches go.mod"
    echo "  - Review logs: /tmp/docker-build.log, /tmp/docker-compose.log"
    exit 1
fi
