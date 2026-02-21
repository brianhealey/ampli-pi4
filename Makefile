.PHONY: build build-pi test lint run-mock run tidy clean deploy deploy-run

BIN_DIR   := ./bin
PI_HOST   := pi@amplipi.local
PI_BIN    := /home/pi/amplipi-go

# ── Local build (arm64, for this machine) ─────────────────────────────────────
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/amplipi ./cmd/amplipi/...

# ── Cross-compile for AmpliPi (Raspberry Pi CM3+, armv7l) ────────────────────
build-pi:
	@mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=arm GOARM=7 go build \
		-ldflags="-s -w" \
		-o $(BIN_DIR)/amplipi-arm ./cmd/amplipi/...
	@echo "Built: $(BIN_DIR)/amplipi-arm (linux/arm)"

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
	scp $(BIN_DIR)/amplipi-arm $(PI_HOST):$(PI_BIN)/amplipi
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
