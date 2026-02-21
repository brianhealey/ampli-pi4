.PHONY: build test lint run-mock run tidy clean

BIN_DIR := ./bin

# Build all binaries
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/amplipi ./cmd/amplipi/...

# Run all tests with race detector
test:
	go test -race ./...

# Lint: use golangci-lint if available, otherwise go vet
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, running go vet instead"; \
		go vet ./...; \
	fi

# Run in mock mode (no hardware required)
run-mock: build
	$(BIN_DIR)/amplipi --mock --addr :8080

# Run with real hardware (requires I2C access)
run: build
	$(BIN_DIR)/amplipi

# Tidy go modules
tidy:
	go mod tidy

# Remove compiled binaries
clean:
	rm -rf $(BIN_DIR)
