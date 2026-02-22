# Docker Infrastructure Verification Guide

**Purpose**: Verify Phases 1-2 (Setup + Foundational) are working correctly

**Last Updated**: 2026-02-22

---

## Prerequisites

- Docker Engine 24+ installed
- Docker Compose v2+ installed
- Make installed
- ~5GB free disk space (for images)

---

## Verification Checklist

### ✅ Phase 1: File Structure Verification

**What to check**: All directories and files exist with correct structure

```bash
# Run from amplipi-go/ directory
cd amplipi-go

# Check directory structure
ls -la docker/
ls -la deployments/
ls -la releases/

# Expected output:
# docker/ should contain: 7 Dockerfiles + docker-compose.yml
# deployments/ should contain: docker-compose.prod.yml, docker-compose.macvlan.yml, .env.example
# releases/ should be empty (ready for manifest files)

# Verify specific files exist
test -f .dockerignore && echo "✅ .dockerignore exists"
test -f docker/Dockerfile.amplipi && echo "✅ Main Dockerfile exists"
test -f docker/docker-compose.yml && echo "✅ Dev compose file exists"
test -f deployments/docker-compose.prod.yml && echo "✅ Prod compose file exists"
test -f deployments/.env.example && echo "✅ Environment template exists"
```

**Expected Result**: All files present, no errors

---

### ✅ Phase 2.1: Dockerfile Syntax Validation

**What to check**: All Dockerfiles have valid syntax

```bash
# Validate each Dockerfile without building
for dockerfile in docker/Dockerfile.*; do
    echo "Validating $dockerfile..."
    docker build -f "$dockerfile" --no-cache --target builder -q . > /dev/null 2>&1 && \
        echo "✅ $dockerfile syntax valid" || \
        echo "❌ $dockerfile has errors"
done
```

**Expected Result**: All Dockerfiles pass syntax validation

---

### ✅ Phase 2.2: Compose File Validation

**What to check**: Docker Compose files have valid syntax

```bash
# Validate development compose file
cd docker
docker compose config > /dev/null && \
    echo "✅ docker-compose.yml is valid" || \
    echo "❌ docker-compose.yml has errors"

# Validate production compose file
cd ../deployments
docker compose -f docker-compose.prod.yml config > /dev/null && \
    echo "✅ docker-compose.prod.yml is valid" || \
    echo "❌ docker-compose.prod.yml has errors"

# Validate macvlan overlay
docker compose -f docker-compose.prod.yml -f docker-compose.macvlan.yml config > /dev/null && \
    echo "✅ macvlan overlay is valid" || \
    echo "❌ macvlan overlay has errors"
```

**Expected Result**: All compose files validate successfully

---

### ✅ Phase 2.3: Build Main Application Image

**What to check**: Main application image builds successfully

```bash
cd amplipi-go

# Build main application (most critical)
time docker build -f docker/Dockerfile.amplipi -t amplipi:test .

# Expected output:
# - Build completes without errors
# - Build time: ~2-5 minutes (first build, ~30s cached)
# - Final image created

# Verify image exists
docker images | grep amplipi

# Check image size (should be reasonable)
docker inspect amplipi:test --format='{{.Size}}' | numfmt --to=iec

# Expected size: 50-150MB (Alpine-based)
```

**Expected Result**: Image builds successfully, reasonable size

---

### ✅ Phase 2.4: Build All Images (Parallel)

**What to check**: All 7 images can be built

```bash
cd amplipi-go

# Build all images using Makefile
make docker-build

# This will build:
# 1. amplipi (main app)
# 2. amplipi-display
# 3. amplipi-airplay
# 4. amplipi-spotify
# 5. amplipi-pandora
# 6. amplipi-lms
# 7. amplipi-dlna

# Verify all images exist
docker images | grep -E "(amplipi|airplay|spotify|pandora|lms|dlna)"

# Expected output: 7 images with 'local' tag
```

**Expected Result**: All 7 images build successfully

**Note**: Some images take longer (airplay ~5-10min, others ~2-5min each)

---

### ✅ Phase 2.5: Test Development Environment

**What to check**: Dev compose stack starts successfully (mock mode)

```bash
cd amplipi-go/docker

# Start development environment (main + display only)
docker compose up -d

# Check containers are running
docker compose ps

# Expected output:
# amplipi-dev       running (healthy)
# amplipi-display-dev   running

# Check logs for errors
docker compose logs amplipi | tail -20
docker compose logs amplipi-display | tail -20

# Test health check
curl http://localhost/api/info

# Expected response: JSON with system info

# Check main app responds
curl -I http://localhost

# Expected: HTTP 200 OK

# Cleanup
docker compose down
```

**Expected Result**:
- Both containers start
- Main app is healthy
- API responds
- No critical errors in logs

---

### ✅ Phase 2.6: Test Optional Services

**What to check**: Streaming services start with --profile flag

```bash
cd amplipi-go/docker

# Start with streaming profile (includes spotify, pandora, lms, dlna)
docker compose --profile streaming up -d

# Check all containers running
docker compose ps

# Expected: 6 containers (main, display, spotify, pandora, lms, dlna)

# Verify each service started
docker compose logs spotify | grep -i "started\|ready\|listening" || echo "Check spotify logs"
docker compose logs pandora | grep -i "started\|ready\|listening" || echo "Check pandora logs"

# Cleanup
docker compose --profile streaming down
```

**Expected Result**: All streaming services start without crashing

---

### ✅ Phase 2.7: Validate Environment Variables

**What to check**: .env.example is complete and correct

```bash
cd amplipi-go/deployments

# Check all required variables are defined
grep -E "^[A-Z_]+=.*$" .env.example | wc -l

# Expected: 20+ variables

# Verify key sections exist
grep -q "AMPLIPI_IMAGE" .env.example && echo "✅ Image config present"
grep -q "MACVLAN_PARENT" .env.example && echo "✅ Network config present"
grep -q "AIRPLAY1_IP" .env.example && echo "✅ AirPlay IPs present"
grep -q "LOG_LEVEL" .env.example && echo "✅ App config present"

# Test compose file substitution
cp .env.example .env
docker compose -f docker-compose.prod.yml config | grep -q "192.168.1.100" && \
    echo "✅ Environment substitution works"
rm .env
```

**Expected Result**: All variable sections present, substitution works

---

### ✅ Phase 2.8: Validate Makefile Targets

**What to check**: All Docker-related Makefile targets exist and work

```bash
cd amplipi-go

# Check targets are defined
grep -E "^docker-" Makefile

# Expected targets:
# docker-build
# docker-build-arm64
# docker-build-multiarch
# docker-push
# docker-deploy
# docker-stop
# docker-logs
# docker-status

# Test help (if available)
make help 2>/dev/null | grep docker || echo "No help available"

# Verify build target syntax
make -n docker-build | head -5

# Expected: Shows build commands (dry run)
```

**Expected Result**: All targets defined, dry-run shows correct commands

---

### ✅ Phase 2.9: Test Volume Management

**What to check**: Named volumes are created and accessible

```bash
cd amplipi-go/docker

# Start stack to create volumes
docker compose up -d

# List volumes
docker volume ls | grep amplipi

# Expected: amplipi-config, amplipi-logs

# Inspect volume details
docker volume inspect docker_amplipi-config

# Test writing to volume
docker exec amplipi-dev sh -c "echo 'test' > /config/test.txt"
docker exec amplipi-dev cat /config/test.txt

# Expected output: "test"

# Verify tmpfs mount
docker exec amplipi-dev df -h | grep tmpfs

# Expected: /tmp mounted as tmpfs (100M)

# Cleanup
docker compose down
docker volume rm docker_amplipi-config docker_amplipi-logs
```

**Expected Result**: Volumes work correctly, tmpfs mounted

---

### ✅ Phase 2.10: Validate .dockerignore

**What to check**: .dockerignore prevents unnecessary files from being copied

```bash
cd amplipi-go

# Build with verbose output to see what's copied
docker build -f docker/Dockerfile.amplipi -t amplipi:test --progress=plain . 2>&1 | \
    grep "transferring context" | head -1

# Expected: Context size should be reasonable (< 100MB)
# If > 500MB, .dockerignore may not be working

# Check .dockerignore patterns
cat .dockerignore | grep -v "^#" | grep -v "^$"

# Verify key patterns exist
grep -q ".git" .dockerignore && echo "✅ .git excluded"
grep -q "node_modules" .dockerignore && echo "✅ node_modules excluded"
grep -q "*.log" .dockerignore && echo "✅ Logs excluded"
```

**Expected Result**: Context size reasonable, key patterns present

---

### ✅ Phase 2.11: Validate Multi-Architecture Support

**What to check**: Images can be built for ARM64 (Pi) and AMD64 (dev)

```bash
cd amplipi-go

# Check buildx is available
docker buildx version

# Create builder (if not exists)
docker buildx create --name amplipi-builder --use 2>/dev/null || \
    docker buildx use amplipi-builder

# Build main image for ARM64 (Pi target)
docker buildx build \
    --platform linux/arm64 \
    -f docker/Dockerfile.amplipi \
    -t amplipi:arm64-test \
    --load \
    .

# Verify image architecture
docker inspect amplipi:arm64-test --format='{{.Architecture}}'

# Expected: arm64

# Build for AMD64 (dev target)
docker buildx build \
    --platform linux/amd64 \
    -f docker/Dockerfile.amplipi \
    -t amplipi:amd64-test \
    --load \
    .

# Verify image architecture
docker inspect amplipi:amd64-test --format='{{.Architecture}}'

# Expected: amd64
```

**Expected Result**: Both architectures build successfully

---

### ✅ Phase 2.12: Test Production Compose (Local)

**What to check**: Production compose can start locally (without hardware)

```bash
cd amplipi-go/deployments

# Copy environment template
cp .env.example .env

# Override hardware mock for local testing
echo "HARDWARE_MOCK=true" >> .env

# Override images to use local builds
sed -i '' 's|ghcr.io/brianhealey/|local-|g' .env

# Try starting production stack (will fail on hardware, but should parse)
docker compose -f docker-compose.prod.yml config > /dev/null && \
    echo "✅ Production compose configuration is valid"

# Note: Don't actually start (no hardware available)
# This just validates the compose file structure

# Cleanup
rm .env
```

**Expected Result**: Configuration validates, no syntax errors

---

## Quick Verification Script

Run all checks in one command:

```bash
#!/bin/bash
# quick-verify.sh - Run all foundational verification checks

cd amplipi-go

echo "=== Docker Infrastructure Verification ==="
echo ""

echo "1. File Structure..."
test -f .dockerignore && test -f docker/Dockerfile.amplipi && \
    test -f docker/docker-compose.yml && \
    test -f deployments/docker-compose.prod.yml && \
    echo "✅ All files present" || echo "❌ Files missing"

echo ""
echo "2. Compose Validation..."
cd docker && docker compose config > /dev/null 2>&1 && \
    echo "✅ Dev compose valid" || echo "❌ Dev compose invalid"
cd ../deployments && docker compose -f docker-compose.prod.yml config > /dev/null 2>&1 && \
    echo "✅ Prod compose valid" || echo "❌ Prod compose invalid"

echo ""
echo "3. Build Main Image..."
cd ..
docker build -f docker/Dockerfile.amplipi -t amplipi:verify-test . > /dev/null 2>&1 && \
    echo "✅ Main image builds" || echo "❌ Build failed"

echo ""
echo "4. Start Dev Environment..."
cd docker
docker compose up -d > /dev/null 2>&1
sleep 5
docker compose ps | grep -q "running" && \
    echo "✅ Dev environment started" || echo "❌ Failed to start"

echo ""
echo "5. Test API..."
curl -sf http://localhost/api/info > /dev/null && \
    echo "✅ API responds" || echo "❌ API not responding"

echo ""
echo "6. Cleanup..."
docker compose down > /dev/null 2>&1
echo "✅ Cleanup complete"

echo ""
echo "=== Verification Complete ==="
```

Save this as `amplipi-go/scripts/verify-docker.sh`, make executable, and run:

```bash
chmod +x amplipi-go/scripts/verify-docker.sh
./amplipi-go/scripts/verify-docker.sh
```

---

## Expected Issues & Solutions

### Issue: "go.mod requires go >= 1.26.0"
**Solution**: Dockerfiles use Go 1.26+ (already fixed)

### Issue: "web/dist not found"
**Solution**: Build web assets first: `make web-build`

### Issue: "context size too large"
**Solution**: Check .dockerignore is present and working

### Issue: "Cannot connect to Docker daemon"
**Solution**: Start Docker Desktop / Docker Engine

### Issue: Build takes very long (>15 min)
**Cause**: First build, downloading all dependencies
**Solution**: Subsequent builds will use cache (~2-5 min)

### Issue: "port 80 already in use"
**Solution**: Stop other services using port 80, or change port in compose file

---

## Success Criteria

All checks should pass:

- ✅ All 7 Dockerfiles build successfully
- ✅ Both compose files validate
- ✅ Dev environment starts and API responds
- ✅ Volumes mount correctly
- ✅ Environment substitution works
- ✅ Multi-arch builds work
- ✅ Makefile targets execute

If all checks pass, **Phase 2 (Foundational) is verified complete** and ready for user story implementation.

---

## Next Steps After Verification

Once verified, you can:

1. **Test on Raspberry Pi**: Deploy to real hardware
   ```bash
   make docker-deploy
   ```

2. **Implement User Story 1**: Containerized system on Pi
   ```bash
   /speckit.implement --phase 3
   ```

3. **Set up CI/CD**: Automate builds with GitHub Actions (User Story 4)

4. **Customize networking**: Adjust macvlan IPs in `.env` for your network
