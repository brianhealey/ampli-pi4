.PHONY: build build-pi test lint run-mock run tidy clean deploy deploy-run

BIN_DIR   := ./bin
PI_HOST   := pi@amplipi.local
PI_BIN    := /home/pi/amplipi-go

# ── Web UI build ──────────────────────────────────────────────────────────────
web-build:
	@echo "Building web UI..."
	@rm -rf web/dist web/.svelte-kit cmd/amplipi/static
	@cd web && npm install && npm run build
	@echo "Copying web assets to cmd/amplipi/static..."
	@cp -r web/dist cmd/amplipi/static
	@echo "Web UI built successfully"

# ── Local build (arm64, for this machine) ─────────────────────────────────────
build: web-build
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/amplipi ./cmd/amplipi/...

# ── Build for AmpliPi (Raspberry Pi 4, 64-bit Raspberry Pi OS / Debian trixie) ─
# Both host (Turing RK1) and target (Pi 4) are arm64 — no true cross-compilation needed.
build-pi: web-build
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm64 go build \
		-ldflags="-s -w" \
		-o $(BIN_DIR)/amplipi-arm64 ./cmd/amplipi/...
	@echo "Built: $(BIN_DIR)/amplipi-arm64 (linux/arm64)"

# ── Tests (local, race detector) ─────────────────────────────────────────────
test:
	CGO_ENABLED=1 go test -race ./...

test-short:
	go test ./...

# ── Lint ──────────────────────────────────────────────────────────────────────
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, running go vet"; \
		go vet ./...; \
	fi

# ── Run locally ───────────────────────────────────────────────────────────────
run-mock: build
	$(BIN_DIR)/amplipi --mock --addr :8080

run: build
	$(BIN_DIR)/amplipi

# ── Deploy to AmpliPi ─────────────────────────────────────────────────────────
# Copies the arm binary to the Pi and uses deployment script
deploy: build-pi scripts-sync
	scp $(BIN_DIR)/amplipi-arm64 $(PI_HOST):/tmp/amplipi-arm64
	ssh $(PI_HOST) 'cd $(PI_BIN) && ./scripts/deploy.sh /tmp/amplipi-arm64 --stop'
	@echo "Deployed to $(PI_HOST):$(PI_BIN)/amplipi"

# Deploy and restart systemd service
deploy-run: build-pi scripts-sync
	scp $(BIN_DIR)/amplipi-arm64 $(PI_HOST):/tmp/amplipi-arm64
	ssh $(PI_HOST) 'cd $(PI_BIN) && ./scripts/deploy.sh /tmp/amplipi-arm64 --restart'

# Alias for deploy-run (systemd service runs with real hardware by default)
deploy-run-hw: deploy-run

# Stop the systemd service on the Pi
deploy-stop:
	ssh $(PI_HOST) 'cd $(PI_BIN) && ./scripts/deploy.sh /tmp/amplipi --stop'

# Restart the systemd service on the Pi
deploy-restart:
	ssh $(PI_HOST) 'sudo systemctl restart amplipi.service && sudo systemctl status amplipi.service --no-pager -l'

# Check status of the systemd service
deploy-status:
	ssh $(PI_HOST) 'sudo systemctl status amplipi.service --no-pager -l'

# Tail logs from the systemd journal
deploy-logs:
	ssh $(PI_HOST) 'sudo journalctl -u amplipi.service -f'

# ── Go modules ───────────────────────────────────────────────────────────────
tidy:
	go mod tidy

# ── Clean ─────────────────────────────────────────────────────────────────────
clean:
	rm -rf $(BIN_DIR)

# ── Pi Setup (run on the Pi via SSH, or directly on device) ──────────────────
.PHONY: setup-pi scripts-sync

# Run the full device setup on the Pi (interactive, requires sudo on the Pi)
setup-pi:
	ssh -t $(PI_HOST) 'cd $(PI_BIN) && sudo scripts/setup.sh'

# Sync scripts/ to Pi without running them
# Useful for iterating on scripts before committing
scripts-sync:
	ssh $(PI_HOST) 'mkdir -p $(PI_BIN)/scripts'
	rsync -av scripts/ $(PI_HOST):$(PI_BIN)/scripts/
	ssh $(PI_HOST) 'chmod +x $(PI_BIN)/scripts/setup.sh $(PI_BIN)/scripts/lib/*.sh'
