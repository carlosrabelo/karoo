# Makefile for the Karoo project

# Default target: show help when running plain `make`
.DEFAULT_GOAL := help

help:
	@echo "Karoo - Stratum V1 Proxy (Go)"
	@echo ""
	@echo "Available targets:"
	@echo "  make help        # show this help (default target)"
	@echo "  make build       # compile static binary ($(BINARY))"
	@echo "  make build-nostatic # compile dynamic binary ($(BINARY_NOSTATIC))"
	@echo "  make clean       # remove binary"
	@echo "  make run         # execute using ./config.json"
	@echo "  make docker      # build Docker image (karoo:latest)"
	@echo "  make install     # install binary to $(INSTALL_BIN_DIR) and config to $(INSTALL_CONFIG_DIR)"
	@echo "  make systemd     # install systemd unit in /etc/systemd/system/karoo.service"
	@echo ""
	@echo "Useful vars: VERSION=$(VERSION) BUILD_TIME=$(BUILD_TIME)"

BINARY=build/karoo
BUILD_DIR=$(dir $(BINARY))
BINARY_NOSTATIC=$(BUILD_DIR)karoo-nostatic
INSTALL_BIN_DIR?=/usr/local/bin
INSTALL_CONFIG_DIR?=/etc/karoo
CONFIG_SOURCE?=config.example.json
CONFIG_DEST=$(INSTALL_CONFIG_DIR)/config.json
SRC=main.go
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME=$(shell date +%Y-%m-%dT%H:%M:%S%z)
LDFLAGS=-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

.PHONY: all build clean run docker systemd install build-nostatic

all: build

build:
	@echo "Building static $(BINARY)..."
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags "$(LDFLAGS)" -o $(BINARY) $(SRC)

build-nostatic:
	@echo "Building dynamic $(BINARY_NOSTATIC)..."
	mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY_NOSTATIC) $(SRC)

clean:
	@echo "Cleaning..."
	rm -f $(BINARY) $(BINARY_NOSTATIC)

run: build
	./$(BINARY) -config ./config.json

# Minimal Docker image build
DOCKER_IMAGE=karoo:latest

docker:
	@echo "Building Docker image $(DOCKER_IMAGE)..."
	docker build -t $(DOCKER_IMAGE) .

# systemd unit installation (requires root)
SYSTEMD_PATH=/etc/systemd/system/karoo.service

install: build
	@echo "Installing binary to $(INSTALL_BIN_DIR)..."
	install -d $(INSTALL_BIN_DIR)
	install -m 755 $(BINARY) $(INSTALL_BIN_DIR)/karoo
	@echo "Installing configuration under $(INSTALL_CONFIG_DIR)..."
	install -d $(INSTALL_CONFIG_DIR)
	@if [ -f $(CONFIG_DEST) ]; then \
		install -m 644 $(CONFIG_SOURCE) $(CONFIG_DEST).example; \
		echo "Existing config preserved; refreshed example at $(CONFIG_DEST).example"; \
	else \
		install -m 644 $(CONFIG_SOURCE) $(CONFIG_DEST); \
	fi

systemd: install
	@echo "Installing systemd unit at $(SYSTEMD_PATH)"
	install -m 644 karoo.service $(SYSTEMD_PATH)
	systemctl daemon-reload
	systemctl enable karoo
	@echo "Use 'systemctl start karoo' to start the service."
