# Developer Guide: Local Multi-Arch Builds

## Overview

This guide covers building AmpliPi Docker images locally for development and testing, including multi-architecture builds for both ARM64 (Raspberry Pi) and AMD64 (development machines).

## Prerequisites

### Required Tools

- **Docker Desktop 4.0+** (includes Buildx)
  - macOS: [Download Docker Desktop](https://www.docker.com/products/docker-desktop)
  - Linux: [Install Docker Engine](https://docs.docker.com/engine/install/)
- **Make** (for using Makefile targets)
  - macOS: Included with Xcode Command Line Tools
  - Linux: `sudo apt-get install build-essential`
- **Go 1.21+** (for local Go development)
  - [Download Go](https://go.lang.org/dl/)

### Optional Tools

- **Docker Compose v2+** (for running services locally)
  - Included with Docker Desktop
  - Linux standalone: [Install Compose](https://docs.docker.com/compose/install/)

## Quick Start

### Build All Images (Local Architecture)

```bash
# Build for your current architecture (AMD64 on Mac, ARM64 on Pi)
make docker-build

# Or manually
docker compose -f docker/docker-compose.yml build
```

### Build Specific Image

```bash
# Build main application
docker build -f docker/Dockerfile.amplipi -t amplipi:local .

# Build display driver
docker build -f docker/Dockerfile.display -t amplipi-display:local .

# Build AirPlay service
docker build -f docker/Dockerfile.airplay -t amplipi-airplay:local .
```

### Run Locally

```bash
# Start all services
docker compose -f docker/docker-compose.yml up -d

# View logs
docker compose -f docker/docker-compose.yml logs -f

# Stop services
docker compose -f docker/docker-compose.yml down
```

## Multi-Architecture Builds

### Why Multi-Arch?

- **ARM64:** Production target (Raspberry Pi CM4S)
- **AMD64:** Development and testing (macOS, Linux workstations)

Building for both architectures ensures images work on all target platforms.

### Setup Buildx

Docker Buildx is required for multi-architecture builds:

```bash
# Verify Buildx is available
docker buildx version

# Create and use a new builder
docker buildx create --name amplipi-builder --use

# Bootstrap the builder (downloads QEMU)
docker buildx inspect --bootstrap

# Verify builder supports multiple platforms
docker buildx inspect
```

### Build Multi-Arch Image

#### Single Image

```bash
# Build for both ARM64 and AMD64
docker buildx build \
  --platform linux/arm64,linux/amd64 \
  -f docker/Dockerfile.amplipi \
  -t amplipi:multi \
  .

# Build and push to registry
docker buildx build \
  --platform linux/arm64,linux/amd64 \
  -f docker/Dockerfile.amplipi \
  -t ghcr.io/brianhealey/amplipi:dev \
  --push \
  .
```

**Note:** Multi-arch builds cannot be loaded into Docker with `--load`. You must either:
1. Push to a registry with `--push`
2. Export to a file with `--output type=docker,dest=image.tar`
3. Build for single platform with `--load`

#### All Images

Use the Makefile for convenience:

```bash
# Build all images for ARM64 only
make docker-build-arm64

# Build all images for AMD64 only
make docker-build-amd64

# Build all images for both (pushes to registry)
make docker-build-multi
```

### Build for Specific Architecture

```bash
# Build ARM64 image only (for Pi)
docker buildx build \
  --platform linux/arm64 \
  -f docker/Dockerfile.amplipi \
  -t amplipi:arm64 \
  --load \
  .

# Build AMD64 image only (for local testing)
docker buildx build \
  --platform linux/amd64 \
  -f docker/Dockerfile.amplipi \
  -t amplipi:amd64 \
  --load \
  .
```

## Development Workflow

### 1. Code Changes

Edit Go code, Dockerfiles, or compose files:

```bash
# Example: Edit main application
vim cmd/amplipi/main.go
```

### 2. Local Testing

Run Go tests:

```bash
# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run specific test
go test -run TestSpecificFunction ./internal/package
```

### 3. Build Updated Image

```bash
# Rebuild single image
docker build -f docker/Dockerfile.amplipi -t amplipi:local .

# Or rebuild all images
make docker-build
```

### 4. Test in Containers

```bash
# Start services with new images
docker compose -f docker/docker-compose.yml up -d

# Check logs
docker compose logs -f amplipi

# Test API
curl http://localhost/api/info

# Shell into container for debugging
docker exec -it amplipi sh
```

### 5. Iterate

Repeat steps 1-4 until satisfied, then commit:

```bash
git add .
git commit -m "Description of changes"
git push origin <branch-name>
```

## Dockerfile Best Practices

### Multi-Stage Builds

Use multi-stage builds to keep images small:

```dockerfile
# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o amplipi ./cmd/amplipi

# Stage 2: Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates curl
COPY --from=builder /build/amplipi /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/amplipi"]
```

### Cross-Platform Compatibility

Use `$TARGETARCH` and `$TARGETOS` for platform-specific logic:

```dockerfile
FROM golang:1.21-alpine AS builder
ARG TARGETARCH
ARG TARGETOS
RUN GOARCH=${TARGETARCH} GOOS=${TARGETOS} go build -o app
```

### Minimize Layers

Combine commands to reduce layers:

```dockerfile
# Bad (3 layers)
RUN apk add ca-certificates
RUN apk add curl
RUN apk add bash

# Good (1 layer)
RUN apk --no-cache add ca-certificates curl bash
```

### Cache Dependencies

Copy dependency files before source code:

```dockerfile
# Copy dependency files first (changes rarely)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code after (changes frequently)
COPY . .
RUN go build
```

## Local Registry for Testing

Use a local registry for faster iteration:

### Start Local Registry

```bash
docker run -d -p 5000:5000 --name registry registry:2
```

### Push to Local Registry

```bash
# Tag image for local registry
docker tag amplipi:local localhost:5000/amplipi:latest

# Push to local registry
docker push localhost:5000/amplipi:latest

# Pull from local registry (e.g., on Pi)
docker pull localhost:5000/amplipi:latest
```

### Configure Docker Compose

Update `.env` to use local registry:

```bash
AMPLIPI_IMAGE=localhost:5000/amplipi:latest
DISPLAY_IMAGE=localhost:5000/amplipi-display:latest
```

## Debugging Tips

### Build Issues

**Problem:** Build fails with "exec format error"

**Solution:** Ensure you're building for the correct architecture:
```bash
docker buildx build --platform linux/arm64 ...
```

**Problem:** Build is very slow

**Solutions:**
1. Use BuildKit: `export DOCKER_BUILDKIT=1`
2. Enable layer caching
3. Optimize Dockerfile (combine RUN commands)
4. Use `.dockerignore` to exclude unnecessary files

### Runtime Issues

**Problem:** Container won't start

**Solution:**
```bash
# Check container logs
docker logs <container-name>

# Inspect container
docker inspect <container-name>

# Try running interactively
docker run -it --rm amplipi:local sh
```

**Problem:** Can't access hardware devices

**Solution:**
1. Check device exists: `ls -la /dev/i2c-1`
2. Verify permissions: `ls -l /dev/i2c-1`
3. Add user to group: `sudo usermod -a -G i2c $USER`
4. Use privileged mode (testing only): `--privileged`

### Network Issues

**Problem:** Can't connect to service

**Solution:**
```bash
# Check port mapping
docker ps

# Check if port is actually bound
netstat -an | grep :80

# Test from inside container
docker exec -it amplipi curl http://localhost/api/info
```

## Performance Optimization

### Image Size

Check image sizes:

```bash
docker images | grep amplipi
```

Reduce image size:
1. Use Alpine base images
2. Remove build dependencies in final stage
3. Delete temporary files
4. Use `.dockerignore`

### Build Speed

Improve build speed:
1. Use layer caching
2. Order Dockerfile commands (least to most frequently changed)
3. Use BuildKit
4. Parallelize builds with matrix strategy

### Runtime Performance

Optimize runtime:
1. Use multi-stage builds (smaller images = faster pulls)
2. Minimize container restarts (fix bugs!)
3. Use health checks to detect issues early
4. Monitor resource usage: `docker stats`

## Makefile Targets

The project includes helpful Makefile targets:

```bash
# Build targets
make docker-build         # Build all images (local arch)
make docker-build-arm64   # Build ARM64 images
make docker-build-amd64   # Build AMD64 images
make docker-build-multi   # Build multi-arch (pushes to registry)

# Push targets
make docker-push          # Push all images to registry

# Deployment targets
make docker-deploy        # Build, push, and deploy to Pi
make docker-deploy-pi     # Deploy to Pi (assumes images exist)

# Test targets
make test                 # Run Go tests
make docker-test          # Run Docker Compose smoke test

# Clean targets
make clean                # Clean build artifacts
make docker-clean         # Remove Docker images
```

## CI/CD Integration

Local builds should match CI/CD builds. The CI/CD pipeline uses the same Dockerfiles, so:

1. **Test locally before pushing:**
   ```bash
   make docker-build
   make docker-test
   ```

2. **If CI fails, reproduce locally:**
   ```bash
   # Pull exact same base images
   docker pull golang:1.21-alpine
   docker pull alpine:latest

   # Build with same flags
   docker buildx build --platform linux/arm64,linux/amd64 ...
   ```

3. **Keep Dockerfiles in sync:**
   - Changes to `docker/Dockerfile.*` affect CI/CD
   - Test multi-arch builds locally before pushing

## Advanced Topics

### Custom Base Images

Create custom base images for faster builds:

```dockerfile
# base.Dockerfile
FROM golang:1.21-alpine
RUN apk --no-cache add ca-certificates curl git
# ... install common dependencies
```

Build and push:
```bash
docker build -f base.Dockerfile -t ghcr.io/brianhealey/amplipi-base:latest .
docker push ghcr.io/brianhealey/amplipi-base:latest
```

Use in application Dockerfiles:
```dockerfile
FROM ghcr.io/brianhealey/amplipi-base:latest
# ... application-specific steps
```

### Conditional Builds

Build different variants:

```dockerfile
ARG VARIANT=full
COPY --from=builder /build/amplipi-${VARIANT} /usr/local/bin/amplipi
```

Build:
```bash
docker build --build-arg VARIANT=minimal -t amplipi:minimal .
```

### Build Secrets

Use build secrets for private dependencies:

```dockerfile
# syntax=docker/dockerfile:1
RUN --mount=type=secret,id=github_token \
    git config --global url."https://$(cat /run/secrets/github_token)@github.com/".insteadOf "https://github.com/"
```

Build:
```bash
docker buildx build --secret id=github_token,src=$HOME/.github_token .
```

## Resources

- [Docker Buildx Documentation](https://docs.docker.com/buildx/working-with-buildx/)
- [Multi-Platform Images](https://docs.docker.com/build/building/multi-platform/)
- [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)
- [BuildKit Documentation](https://docs.docker.com/build/buildkit/)
