# Repository Secrets Documentation

## Overview

This document describes the secrets and tokens required for the AmpliPi CI/CD pipeline and how to configure them.

## Required Secrets

### GITHUB_TOKEN (Automatic)

**Purpose:** Authentication for GitHub Container Registry and API access

**Type:** Automatic (no configuration needed)

**Permissions:**
- `contents: read` - Read repository code
- `packages: write` - Push container images to GHCR
- `contents: write` - Create GitHub releases (release workflow)

**How It Works:**
- Automatically provided by GitHub Actions
- Scoped to the repository
- Valid only during workflow execution
- No manual configuration required

**Usage in Workflows:**
```yaml
- name: Log in to GitHub Container Registry
  uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
```

**Limitations:**
- Cannot trigger other workflows (by design)
- Cannot access private repos outside current repo
- Expires after workflow completion

## Optional Secrets

### Personal Access Token (PAT) - For Local Development

**Purpose:** Local authentication to GitHub Container Registry

**Required Scopes:**
- `read:packages` - Pull images from GHCR
- `write:packages` - Push images to GHCR
- `delete:packages` - Delete image versions (optional)

**How to Create:**

1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Click "Generate new token (classic)"
3. Give it a descriptive name: "GHCR Access - Development Machine"
4. Set expiration (recommended: 90 days)
5. Select scopes:
   - ✅ `read:packages`
   - ✅ `write:packages`
   - ✅ `delete:packages` (optional)
6. Click "Generate token"
7. **Copy the token immediately** (you won't see it again)

**How to Use:**

```bash
# Store token in environment variable
export GITHUB_PAT=ghp_xxxxxxxxxxxxxxxxxxxx

# Login to GHCR
echo $GITHUB_PAT | docker login ghcr.io -u YOUR_USERNAME --password-stdin

# Now you can pull/push images
docker pull ghcr.io/brianhealey/amplipi:latest
docker push ghcr.io/brianhealey/amplipi:dev
```

**Security Best Practices:**
- Store PAT in password manager (1Password, LastPass, etc.)
- Never commit PAT to git
- Set expiration dates
- Rotate tokens regularly
- Use minimal required scopes
- Revoke tokens when no longer needed

## No Additional Secrets Needed!

The AmpliPi CI/CD pipeline is designed to work with only the automatic `GITHUB_TOKEN`. You do **not** need to configure any additional secrets in the repository settings.

## Package Permissions

### Making Images Public

By default, packages (container images) published to GHCR are private. To make them public:

1. Go to the repository on GitHub
2. Click on "Packages" (right sidebar)
3. Click on the package name (e.g., `amplipi`)
4. Click "Package settings"
5. Scroll to "Danger Zone"
6. Click "Change visibility"
7. Select "Public"
8. Confirm the change

**Note:** This only needs to be done once per package (image).

### Verifying Package Visibility

Public packages can be pulled without authentication:

```bash
# Should work without docker login if package is public
docker pull ghcr.io/brianhealey/amplipi:latest
```

Private packages require authentication:

```bash
# Requires authentication if package is private
echo $GITHUB_PAT | docker login ghcr.io -u USERNAME --password-stdin
docker pull ghcr.io/brianhealey/amplipi:latest
```

## Workflow Permissions

Each workflow has specific permissions configured:

### Build Workflow
```yaml
permissions:
  contents: read     # Read source code
  packages: write    # Push images to GHCR
```

### Test Workflow
```yaml
# Uses default permissions (contents: read)
```

### Release Workflow
```yaml
permissions:
  contents: write    # Create GitHub releases
  packages: write    # Push images to GHCR
```

### Cleanup Workflow
```yaml
permissions:
  packages: write    # Delete old image versions
```

## Deployment to Pi

### SSH Authentication

For automated deployment to Raspberry Pi, you may want to set up SSH key authentication:

**Generate SSH Key:**
```bash
ssh-keygen -t ed25519 -C "github-actions@amplipi"
```

**Add Public Key to Pi:**
```bash
ssh-copy-id pi@amplipi.local
```

**Add Private Key to GitHub Secrets (Optional):**

If you want CI/CD to deploy directly to Pi:

1. Go to repository Settings → Secrets and variables → Actions
2. Click "New repository secret"
3. Name: `PI_SSH_KEY`
4. Value: Contents of private key file (`cat ~/.ssh/id_ed25519`)
5. Click "Add secret"

**Use in Workflow:**
```yaml
- name: Deploy to Pi
  env:
    SSH_KEY: ${{ secrets.PI_SSH_KEY }}
  run: |
    echo "$SSH_KEY" > /tmp/ssh_key
    chmod 600 /tmp/ssh_key
    ssh -i /tmp/ssh_key pi@amplipi.local 'docker-compose up -d'
```

**Note:** Currently, the project uses manual deployment scripts (`make docker-deploy`) rather than automated deployment from CI/CD.

## Environment Variables

The following environment variables can be configured in `.env` files (not secrets):

### Build-Time Variables
- `AMPLIPI_IMAGE` - Override default image reference
- `DISPLAY_IMAGE` - Override display driver image
- `AIRPLAY_IMAGE` - Override AirPlay image
- (etc.)

### Runtime Variables
- `LOG_LEVEL` - Application log level (debug, info, warn, error)
- `HARDWARE_MOCK` - Enable mock mode for testing without hardware
- `DISPLAY_TYPE` - Display type (tft, eink)
- `MACVLAN_PARENT` - Network interface for macvlan
- `MACVLAN_SUBNET` - Subnet for macvlan network
- `MACVLAN_GATEWAY` - Gateway for macvlan network
- `AIRPLAY1_IP` through `AIRPLAY4_IP` - Static IPs for AirPlay instances

These are stored in `.env.example` and customized per deployment.

## Security Checklist

- [ ] Never commit secrets to git
- [ ] Use `.env` for non-sensitive configuration only
- [ ] Keep PATs in password manager
- [ ] Set expiration on PATs
- [ ] Use minimal required scopes
- [ ] Rotate tokens regularly
- [ ] Review repository permissions quarterly
- [ ] Enable 2FA on GitHub account
- [ ] Use branch protection rules
- [ ] Require review for sensitive changes

## Troubleshooting

### "permission denied" when pushing images

**Issue:** CI/CD fails with "permission denied" when pushing to GHCR

**Solutions:**
1. Check workflow has `packages: write` permission
2. Verify GITHUB_TOKEN is being used correctly
3. Check package visibility settings
4. Ensure repository owner matches image path

### "unauthorized" when pulling images

**Issue:** Cannot pull images from GHCR

**Solutions:**
1. Check if package is private (requires authentication)
2. Verify you're logged in: `docker login ghcr.io`
3. Check PAT has `read:packages` scope
4. Ensure PAT hasn't expired

### Build succeeds but images not visible

**Issue:** Build workflow succeeds but images don't appear in GHCR

**Solutions:**
1. Check if building on PR (PRs don't push by default)
2. Verify `push: true` in build step
3. Check branch name matches workflow trigger
4. Look for "Skipping push" in workflow logs

### Cannot create release

**Issue:** Release workflow fails with "permission denied"

**Solutions:**
1. Check workflow has `contents: write` permission
2. Verify tag matches pattern (`v*`)
3. Ensure you have admin/maintain role on repository
4. Check branch protection rules

## Additional Resources

- [GitHub Actions Permissions](https://docs.github.com/en/actions/security-guides/automatic-token-authentication)
- [GitHub Container Registry Authentication](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [Personal Access Tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token)
- [Docker Login Documentation](https://docs.docker.com/engine/reference/commandline/login/)
