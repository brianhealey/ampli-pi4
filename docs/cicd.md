# CI/CD Pipeline Documentation

## Overview

AmpliPi uses GitHub Actions for automated building, testing, and releasing of Docker container images. The pipeline is designed to:

- Build multi-architecture images (ARM64 + AMD64) for all services
- Run automated tests on every commit and pull request
- Publish images to GitHub Container Registry (ghcr.io)
- Create releases with manifests and deployment files
- Clean up old container images automatically

## Workflows

### 1. Build Workflow (`.github/workflows/build.yml`)

**Triggers:**
- Push to `main`, `master`, or `001-docker-migration` branches
- Pull requests to `main` or `master`

**Actions:**
1. Sets up QEMU for cross-platform emulation
2. Configures Docker Buildx for multi-arch builds
3. Logs into GitHub Container Registry (on push only, not PR)
4. Builds all 7 container images in parallel:
   - `amplipi` - Main application
   - `amplipi-display` - Display driver
   - `amplipi-airplay` - AirPlay2 service
   - `amplipi-spotify` - Spotify Connect
   - `amplipi-pandora` - Pandora streaming
   - `amplipi-lms` - Logitech Media Server client
   - `amplipi-dlna` - DLNA renderer
5. Pushes images to GHCR (on push only)
6. Uses GitHub Actions cache for faster subsequent builds

**Image Tags:**
- `<branch-name>` - Branch builds (e.g., `main`, `001-docker-migration`)
- `<branch-name>-<sha>` - Commit-specific builds
- `latest` - Latest build from default branch
- PR builds are tagged with `pr-<number>`

**Build Time:**
- Initial build: ~15-20 minutes (all images)
- Subsequent builds with cache: ~5-10 minutes

### 2. Test Workflow (`.github/workflows/test.yml`)

**Triggers:**
- Push to `main`, `master`, or `001-docker-migration` branches
- Pull requests to `main` or `master`

**Actions:**

#### Go Tests
1. Sets up Go 1.21
2. Downloads dependencies
3. Runs tests with race detector: `go test -v -race -coverprofile=coverage.out ./...`
4. Generates coverage report
5. Uploads coverage artifact (available for 7 days)

#### Docker Compose Smoke Test
1. Builds all images locally
2. Starts services with `docker-compose.yml`
3. Waits for health checks to pass (60s timeout)
4. Tests API endpoint: `GET /api/info`
5. Shows logs on failure
6. Cleans up containers and volumes

**Test Time:**
- Go tests: ~2-5 minutes
- Docker Compose test: ~5-10 minutes

### 3. Release Workflow (`.github/workflows/release.yml`)

**Triggers:**
- Push of tags matching `v*` pattern (e.g., `v1.0.0`)

**Actions:**
1. Extracts version from tag
2. Builds and pushes all images with version tag + `latest`
3. Generates release manifest using `scripts/generate-manifest.sh`
4. Bundles deployment files:
   - `manifest.yml`
   - `docker-compose.prod.yml`
   - `.env.example`
   - Deployment scripts
5. Creates tarball: `amplipi-docker-<version>.tar.gz`
6. Generates release notes
7. Creates GitHub Release with all assets

**Release Assets:**
- `manifest.yml` - Release manifest for automated updates
- `docker-compose.prod.yml` - Production compose file
- `.env.example` - Environment variables template
- `amplipi-docker-<version>.tar.gz` - Complete deployment bundle

**Image Tags:**
- `v<version>` - Semantic version tag (e.g., `v1.0.0`)
- `latest` - Updated to point to this release

### 4. Cleanup Workflow (`.github/workflows/cleanup.yml`)

**Triggers:**
- Weekly schedule: Every Sunday at 2:00 AM UTC
- Manual dispatch via GitHub Actions UI

**Actions:**
1. Deletes untagged image versions (keeps last 10)
2. Deletes old tagged versions except:
   - `latest` tag
   - Semantic version tags (e.g., `v1.0.0`)
   - Last 5 versions

**Purpose:**
- Prevents unlimited storage growth
- Keeps recent development builds for debugging
- Preserves all release versions

## Container Registry

**Registry:** GitHub Container Registry (ghcr.io)

**Image Naming:**
```
ghcr.io/<owner>/<image>:<tag>
```

**Example:**
```
ghcr.io/brianhealey/amplipi:v1.0.0
ghcr.io/brianhealey/amplipi:latest
ghcr.io/brianhealey/amplipi:main
ghcr.io/brianhealey/amplipi:main-abc123
```

**Visibility:**
- Images are private by default
- Can be made public via Package Settings
- Authentication required for private images

**Authentication:**
- CI/CD: Automatic via `GITHUB_TOKEN`
- Local: Use Personal Access Token (PAT)
  ```bash
  echo $GITHUB_PAT | docker login ghcr.io -u USERNAME --password-stdin
  ```

## Layer Caching

The build workflow uses GitHub Actions cache to speed up builds:

**Cache Key:** `type=gha,scope=<image-name>`

**How It Works:**
1. First build pushes layer cache to GitHub Actions
2. Subsequent builds pull cached layers
3. Only changed layers are rebuilt
4. Dramatically reduces build time (60-80% faster)

**Cache Scope:**
- Separate cache per image (e.g., `amplipi`, `amplipi-display`)
- Scoped to repository
- Automatically managed by GitHub

## Multi-Architecture Builds

All images are built for two architectures:

1. **linux/arm64** - Raspberry Pi CM4S (primary target)
2. **linux/amd64** - Development machines (macOS, Linux)

**How It Works:**
- QEMU emulates ARM64 on AMD64 runners
- Docker Buildx builds both architectures in one command
- Images are multi-arch manifests (single tag, multiple platforms)

**Pulling Images:**
Docker automatically selects the correct architecture:
```bash
# On Pi (ARM64): pulls linux/arm64 image
docker pull ghcr.io/brianhealey/amplipi:latest

# On Mac (AMD64): pulls linux/amd64 image
docker pull ghcr.io/brianhealey/amplipi:latest
```

## Creating a Release

### 1. Prepare Release

Ensure all changes are committed and pushed:
```bash
git add .
git commit -m "Prepare release v1.0.0"
git push origin main
```

### 2. Create and Push Tag

```bash
# Create annotated tag
git tag -a v1.0.0 -m "Release v1.0.0: Docker migration"

# Push tag to GitHub
git push origin v1.0.0
```

### 3. Monitor Release Workflow

1. Go to GitHub Actions tab
2. Watch "Release" workflow run
3. Wait for all images to build (~15-20 minutes)
4. Verify release is created with assets

### 4. Verify Release

Check that release includes:
- ✅ Release notes
- ✅ `manifest.yml`
- ✅ `docker-compose.prod.yml`
- ✅ `.env.example`
- ✅ `amplipi-docker-v1.0.0.tar.gz`

Verify images are published:
```bash
docker pull ghcr.io/brianhealey/amplipi:v1.0.0
docker pull ghcr.io/brianhealey/amplipi-display:v1.0.0
# ... etc
```

### 5. Test Deployment

Deploy to Pi and verify:
```bash
# Download release
curl -LO https://github.com/brianhealey/amplipi/releases/download/v1.0.0/amplipi-docker-v1.0.0.tar.gz

# Extract and deploy
tar xzf amplipi-docker-v1.0.0.tar.gz
scp -r * pi@amplipi.local:/home/pi/amplipi-docker/

# Start services
ssh pi@amplipi.local 'cd /home/pi/amplipi-docker && sudo docker compose -f docker-compose.prod.yml up -d'
```

## Troubleshooting

### Build Failures

**Issue:** Multi-arch build fails with "cannot find manifest"

**Solution:**
1. Ensure Buildx is set up correctly
2. Check QEMU is installed
3. Verify Dockerfile is compatible with both architectures

**Issue:** Build timeout (>6 hours)

**Solution:**
1. Optimize Dockerfile (use multi-stage builds)
2. Reduce image size
3. Check for cache issues

### Push Failures

**Issue:** "denied: permission_denied"

**Solution:**
1. Check `packages: write` permission in workflow
2. Verify GITHUB_TOKEN is valid
3. Check repository package settings

**Issue:** Rate limit exceeded

**Solution:**
1. Wait for rate limit to reset
2. Use layer caching to reduce pulls
3. Consider using GitHub Actions cache

### Test Failures

**Issue:** Health check timeout

**Solution:**
1. Increase timeout in test workflow
2. Check container logs for startup issues
3. Verify `HARDWARE_MOCK=true` is set

**Issue:** API endpoint test fails

**Solution:**
1. Check port mapping (80:80)
2. Verify container is running
3. Check firewall settings

## Best Practices

1. **Always run tests before releasing:**
   - Push to branch first
   - Wait for CI to pass
   - Then create release tag

2. **Use semantic versioning:**
   - `v1.0.0` - Major release
   - `v1.1.0` - Minor release
   - `v1.0.1` - Patch release

3. **Write clear release notes:**
   - List new features
   - Document breaking changes
   - Include migration steps

4. **Test releases on Pi before announcing:**
   - Deploy to test device
   - Verify all services work
   - Check AirPlay discovery
   - Test web interface

5. **Monitor image sizes:**
   - Keep images as small as possible
   - Use Alpine base images
   - Remove unnecessary files

## Metrics

Track these metrics for CI/CD health:

- **Build success rate:** Target >95%
- **Build time:** Target <15 minutes (with cache)
- **Test success rate:** Target 100%
- **Image size:** Target <200MB per image
- **Release frequency:** As needed (monthly recommended)

## Future Improvements

- [ ] Add automated security scanning (Trivy, Snyk)
- [ ] Implement canary deployments
- [ ] Add performance benchmarks
- [ ] Create staging environment for testing
- [ ] Add automated rollback on failed health checks
- [ ] Implement blue-green deployments
