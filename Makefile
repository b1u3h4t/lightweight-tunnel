.PHONY: all build clean install test help

# Binary name
BINARY_NAME=lightweight-tunnel
OUTPUT_DIR=bin
SERVICE_NAME?=$(BINARY_NAME)
CONFIG_PATH?=/etc/lightweight-tunnel/config.json
INSTALL_BIN_DIR=/usr/local/bin
SYSTEMD_UNIT=/etc/systemd/system/$(SERVICE_NAME).service

# Build variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/$(OUTPUT_DIR)
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

all: clean build

## build: Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)
	@echo "Build complete: $(GOBIN)/$(BINARY_NAME)"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(OUTPUT_DIR)
	@echo "Clean complete"

## install: Install dependencies
install:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies installed"

## test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

## install-service: Install systemd service (CONFIG_PATH=/path/to/config.json SERVICE_NAME=name)
install-service: build
	@if [ -z "$(CONFIG_PATH)" ]; then \
		echo "ERROR: CONFIG_PATH is required. Example: make install-service CONFIG_PATH=/etc/lightweight-tunnel/config.json"; \
		exit 1; \
	fi
	@echo "Installing binary to $(INSTALL_BIN_DIR)..."
	@sudo install -m 755 $(GOBIN)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Creating systemd unit $(SYSTEMD_UNIT)..."
	@sudo tee $(SYSTEMD_UNIT) > /dev/null <<-'EOF'
	[Unit]
	Description=Lightweight Tunnel Service ($(SERVICE_NAME))
	After=network-online.target
	Wants=network-online.target

	[Service]
	Type=simple
	ExecStart=$(INSTALL_BIN_DIR)/$(BINARY_NAME) -c $(CONFIG_PATH)
	Restart=on-failure
	RestartSec=5s
	User=root
	AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
	CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW

	[Install]
	WantedBy=multi-user.target
	EOF
	@sudo systemctl daemon-reload
	@sudo systemctl enable $(SERVICE_NAME)
	@echo "Service installed. Start it with: sudo systemctl start $(SERVICE_NAME)"

## run-server: Run as server (requires root)
run-server: build
	@echo "Running as server..."
	sudo $(GOBIN)/$(BINARY_NAME) -m server

## run-client: Run as client (requires root and SERVER_IP env var)
run-client: build
	@echo "Running as client..."
	@if [ -z "$(SERVER_IP)" ]; then \
		echo "ERROR: Please set SERVER_IP environment variable"; \
		exit 1; \
	fi
	sudo $(GOBIN)/$(BINARY_NAME) -m client -r $(SERVER_IP):9000 -t 10.0.0.2/24

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
