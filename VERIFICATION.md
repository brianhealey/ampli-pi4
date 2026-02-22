# Docker Infrastructure Verification

Quick guide to verify Phases 1-2 (Setup + Foundational) are working correctly.

## Quick Start

### Automated Verification (Recommended)

Run the automated verification script:

```bash
cd amplipi-go
./scripts/verify-docker.sh
```

This will check:
- ✅ File structure (all Dockerfiles, compose files, configs)
- ✅ Docker/Compose installed and running
- ✅ Compose file syntax validation
- ✅ Environment variables configured
- ✅ Makefile targets defined

**Optional tests** (prompts you):
- 🏗️ Build main image (~2-5 min)
- 🚀 Start dev environment and test API

### Expected Output

```
════════════════════════════════════════════════════════════════
  Docker Infrastructure Verification (Phases 1-2)
════════════════════════════════════════════════════════════════

📁 Phase 1: File Structure
────────────────────────────────────────────────────────────────
Checking: Docker directory exists... ✅ PASS
Checking: Deployments directory exists... ✅ PASS
...

🎉 All checks passed! Foundational phase is verified.
```

## Manual Verification

If you prefer step-by-step manual testing, see the comprehensive guide:

📖 **[docs/docker-verification.md](docs/docker-verification.md)**

Covers:
- Phase 1: File structure
- Phase 2: Dockerfile validation
- Phase 3: Compose validation
- Phase 4: Environment variables
- Phase 5: Volume management
- Phase 6: Multi-architecture builds
- Phase 7: Dev environment testing
- Common issues & troubleshooting

## Quick Smoke Tests

### 1. Build Main Image

```bash
cd amplipi-go
docker build -f docker/Dockerfile.amplipi -t amplipi:test .
```

**Expected**: Build succeeds in 2-5 minutes

### 2. Start Dev Environment

```bash
cd amplipi-go/docker
docker compose up -d
docker compose ps
```

**Expected**: 2 containers running (amplipi-dev, amplipi-display-dev)

### 3. Test API

```bash
curl http://localhost/api/info
```

**Expected**: JSON response with system info

### 4. Cleanup

```bash
cd amplipi-go/docker
docker compose down
```

## Common Issues

### "go.mod requires go >= 1.26.0"
✅ **Fixed** - Dockerfiles updated to use Go 1.26

### "web/dist not found"
Run: `make web-build` before building Docker images

### "Docker daemon not running"
Start Docker Desktop or Docker Engine

### "Port 80 already in use"
Stop other services or change port in `docker/docker-compose.yml`:
```yaml
ports:
  - "8080:80"  # Use port 8080 instead
```

## Success Criteria

All of these should work:

- [x] All 7 Dockerfiles exist and have valid syntax
- [x] Dev compose file (`docker/docker-compose.yml`) validates
- [x] Prod compose file (`deployments/docker-compose.prod.yml`) validates
- [x] Environment template (`deployments/.env.example`) is complete
- [x] Main image builds successfully
- [x] Dev environment starts and API responds
- [x] Makefile targets are defined

## Next Steps

Once verified:

1. **Build all images**:
   ```bash
   make docker-build
   ```

2. **Deploy to Raspberry Pi** (requires Pi hardware):
   ```bash
   make docker-deploy
   ```

3. **Continue with User Stories**:
   ```bash
   /speckit.implement  # Continue with Phase 3+
   ```

4. **Customize for your network**:
   - Edit `deployments/.env.example`
   - Set macvlan IPs for your network
   - Configure AirPlay device names

## Getting Help

- **Detailed verification guide**: [docs/docker-verification.md](docs/docker-verification.md)
- **Docker troubleshooting**: Check logs in `/tmp/docker-build.log` and `/tmp/docker-compose.log`
- **Compose issues**: Run `docker compose config` to see expanded configuration
- **Build issues**: Add `--progress=plain` to docker build for verbose output

---

**Status**: Phase 1-2 Complete ✅

**Created**: 2026-02-22
