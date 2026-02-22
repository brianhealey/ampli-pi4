.PHONY: build build-pi test lint run-mock run tidy clean deploy deploy-run

BIN_DIR   := ./bin
PI_HOST   := pi@amplipi.local
PI_BIN    := /home/pi/amplipi-go

# ── Web UI build ──────────────────────────────────────────────────────────────
web-build:
	@echo "Building web UI..."
	@cd web && npm install && npm run build
	@echo "Copying web assets to cmd/amplipi/static..."
	@rm -rf cmd/amplipi/static
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
# Copies the arm binary to the Pi and optionally restarts it.
deploy: build-pi
	scp $(BIN_DIR)/amplipi-arm64 $(PI_HOST):$(PI_BIN)/amplipi
	@echo "Deployed to $(PI_HOST):$(PI_BIN)/amplipi"

# Deploy and run in mock mode (safe — no I2C required)
deploy-run: deploy
	ssh $(PI_HOST) 'kill -9 $$(cat /tmp/amplipi.pid 2>/dev/null) 2>/dev/null; \
		cd $(PI_BIN) && \
		nohup ./amplipi --mock --addr :8080 > /tmp/amplipi.log 2>&1 & \
		echo $$! > /tmp/amplipi.pid && \
		echo "Running PID $$(cat /tmp/amplipi.pid)"'

# Deploy and run with real hardware
deploy-run-hw: deploy
	ssh $(PI_HOST) 'kill -9 $$(cat /tmp/amplipi.pid 2>/dev/null) 2>/dev/null; \
		cd $(PI_BIN) && \
		nohup ./amplipi --addr :8080 > /tmp/amplipi.log 2>&1 & \
		echo $$! > /tmp/amplipi.pid && \
		echo "Running PID $$(cat /tmp/amplipi.pid)"'

# Stop the running binary on the Pi
deploy-stop:
	ssh $(PI_HOST) 'kill $$(cat /tmp/amplipi.pid 2>/dev/null) 2>/dev/null && echo stopped || echo "not running"'

# Tail logs from the Pi
deploy-logs:
	ssh $(PI_HOST) 'tail -f /tmp/amplipi.log'

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
	rsync -av --mkpath scripts/ $(PI_HOST):$(PI_BIN)/scripts/
	ssh $(PI_HOST) 'chmod +x $(PI_BIN)/scripts/setup.sh $(PI_BIN)/scripts/lib/*.sh'
