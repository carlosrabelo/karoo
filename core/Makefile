## Karoo Core Module

PROJECT_ROOT        := $(abspath ..)
GO                  := go
SRC                 := ./cmd/karoo
BIN                 := karoo
BUILD_DIR           := $(PROJECT_ROOT)/bin
BINARY              := $(BUILD_DIR)/$(BIN)
CONFIG_TEMPLATE     := $(PROJECT_ROOT)/config/config.example.json
RUN_CONFIG         ?= $(PROJECT_ROOT)/config.json
ROOT_BIN_DIR        ?= /usr/local/bin
ROOT_CONFIG_DIR     ?= /etc/karoo
USER_BIN_DIR        ?= $(HOME)/.local/bin
USER_CONFIG_DIR     ?= $(HOME)/.config/karoo
INSTALL_BIN_DIR     :=
INSTALL_CONFIG_DIR  :=
CONFIG_BASENAME     := config.json
VERSION             := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME          := $(shell date +%Y-%m-%dT%H:%M:%S%z)
LDFLAGS             := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)
GOCACHE            ?= $(PROJECT_ROOT)/.cache/go-build

export GOCACHE

.DEFAULT_GOAL       := help

.PHONY: help build build-all run install _install-internal test test-coverage lint fmt vet deps deps-update mod-tidy clean info quality testing utilities

help:
	@printf "Karoo Core Module\n\n"
	@printf "Build & Install\n"
	@printf "  %-15s %s\n" "build" "Compile Stratum proxy binary"
	@printf "  %-15s %s\n" "install" "Install binary (auto root/user paths)"
	@printf "  %-15s %s\n" "run" "Execute proxy with config.json"
	@printf "  %-15s %s\n" "clean" "Remove binaries and Go caches"
	@printf "\n"
	@printf "Quality\n"
	@printf "  %-15s %s\n" "quality"  "Run all quality checks"
	@printf "  %-15s %s\n" "lint"     "Run golangci-lint when available"
	@printf "  %-15s %s\n" "fmt"      "Format Go sources with gofmt"
	@printf "  %-15s %s\n" "vet"      "Analyze code with go vet"
	@printf "  %-15s %s\n" "mod-tidy" "Run go mod tidy and verify"
	@printf "\n"
	@printf "Testing\n"
	@printf "  %-15s %s\n" "testing"      "Run complete test suite"
	@printf "  %-15s %s\n" "test"         "Run go test ./..."
	@printf "  %-15s %s\n" "test-coverage" "Run tests with coverage report"
	@printf "\n"
	@printf "Utilities\n"
	@printf "  %-15s %s\n" "utilities"   "Run utility commands"
	@printf "  %-15s %s\n" "deps"        "Download Go module dependencies"
	@printf "  %-15s %s\n" "deps-update" "Update Go module dependencies"
	@printf "  %-15s %s\n" "info"        "Show build metadata summary"

build: $(BINARY)

$(BINARY):
	@mkdir -p $(BUILD_DIR) $(GOCACHE)
	CGO_ENABLED=0 $(GO) build -trimpath -tags netgo -ldflags="$(LDFLAGS)" -o $(BINARY) $(SRC)

build-all:
	@mkdir -p $(BUILD_DIR) $(GOCACHE)
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -tags netgo -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN)-linux-amd64 $(SRC)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -tags netgo -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN)-linux-arm64 $(SRC)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -tags netgo -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN)-darwin-amd64 $(SRC)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -tags netgo -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN)-darwin-arm64 $(SRC)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -tags netgo -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BIN)-windows-amd64.exe $(SRC)
	@echo "Multi-platform build completed. Binaries in $(BUILD_DIR)/"

run: $(BINARY)
	@if [ ! -f "$(RUN_CONFIG)" ]; then \
		echo "Config $(RUN_CONFIG) not found. Copy $(CONFIG_TEMPLATE) first."; \
		exit 1; \
	fi
	@$(BUILD_DIR)/$(BIN) -config $(RUN_CONFIG)

install: $(BINARY)
	@set -e; \
	if [ "$$(id -u)" -eq 0 ]; then \
		$(MAKE) INSTALL_BIN_DIR="$(ROOT_BIN_DIR)" _install-internal; \
	else \
		$(MAKE) INSTALL_BIN_DIR="$(USER_BIN_DIR)" _install-internal; \
	fi

_install-internal:
	@set -e; \
	if [ -z "$(INSTALL_BIN_DIR)" ]; then \
		echo "INSTALL_BIN_DIR not set"; \
		exit 2; \
	fi; \
	install -d "$(INSTALL_BIN_DIR)"; \
	install -m 755 $(BUILD_DIR)/$(BIN) "$(INSTALL_BIN_DIR)/$(BIN)"

test:
	$(GO) test ./...

test-coverage:
	@mkdir -p $(BUILD_DIR)
	$(GO) test -coverprofile=$(BUILD_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report generated: $(BUILD_DIR)/coverage.html"

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

deps:
	$(GO) mod download

deps-update:
	$(GO) get -u ./...
	$(GO) mod tidy

mod-tidy:
	$(GO) mod tidy
	$(GO) mod verify

clean:
	@rm -f $(BINARY)
	@$(GO) clean -cache -testcache 2>/dev/null || true

info:
	@printf "Binary        : %s\n" $(BIN)
	@printf "Source        : %s\n" $(SRC)
	@printf "Build path    : %s\n" $(BUILD_DIR)
	@printf "Version       : %s\n" $(VERSION)
	@printf "Build time    : %s\n" $(BUILD_TIME)
	@printf "Go toolchain  : %s\n" "$$($(GO) version)"

quality: lint fmt vet

testing: test test-coverage

utilities: info deps

%:
	@echo "make: *** Unknown target '$@'. Use 'make help' for available targets."; exit 2
