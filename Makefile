MAKEFLAGS += --no-print-directory

.DEFAULT_GOAL := help

.PHONY: build build-all clean deps deps-update deployment docker docker-compose-down \
	docker-compose-logs docker-compose-up fmt help info install \
	install-system k8s-apply k8s-delete k8s-logs k8s-namespace k8s-status lint \
	mod-tidy quality run systemd systemd-logs systemd-status systemd-uninstall \
	test test-coverage testing uninstall uninstall-system utilities vet version

GO         := go
BINARY_NAME := karoo
BUILD_DIR  := $(abspath bin)
BINARY     := $(BUILD_DIR)/$(BINARY_NAME)
RUN_CONFIG ?= config.json
CONFIG_TEMPLATE := config.example.json
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME := $(shell date +%Y-%m-%dT%H:%M:%S%z)
DEPLOY_DIR := deploy

export BINARY_NAME

help: ## Show available targets
	@echo "karoo - Available targets"
	@echo ""
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*## "} {printf "  %-22s %s\n", $$1, $$2}'

## Build & run

build: ## Compile Stratum proxy binary
	@./.make/build.sh

build-all: ## Cross-compile for multiple platforms
	@./.make/build-all.sh

run: build ## Execute proxy with config.json
	@if [ ! -f "$(RUN_CONFIG)" ]; then \
		echo "Config $(RUN_CONFIG) not found. Copy $(CONFIG_TEMPLATE) first."; \
		exit 1; \
	fi
	@$(BINARY) -config $(RUN_CONFIG)

install: ## Install to ~/.local/bin (requires prior make build)
	@./.make/install.sh

install-system: ## Install to /usr/local/bin (requires prior make build; sudo only for copy)
	@SYSTEM=1 ./.make/install.sh

uninstall: ## Remove from ~/.local/bin (sudo only if needed)
	@./.make/uninstall.sh

uninstall-system: ## Remove from /usr/local/bin (sudo only if needed)
	@SYSTEM=1 ./.make/uninstall.sh

## Test

test: ## Run go test ./...
	@./.make/test.sh

test-coverage: ## Run tests with coverage report
	@./.make/test-coverage.sh

testing: test test-coverage ## Run complete test suite

## Quality

lint: ## Run golangci-lint when available
	@./.make/lint.sh

fmt: ## Format Go sources with gofmt
	$(GO) fmt ./...

vet: ## Analyze code with go vet
	$(GO) vet ./...

mod-tidy: ## Run go mod tidy and verify
	$(GO) mod tidy
	$(GO) mod verify

quality: fmt lint ## Run all quality checks

## Utilities

deps: ## Download Go module dependencies
	$(GO) mod download

deps-update: ## Update Go module dependencies
	$(GO) get -u ./...
	$(GO) mod tidy

version: ## Show version and build time
	@echo "$(BINARY_NAME) $(VERSION) ($(BUILD_TIME))"

info: ## Show build metadata summary
	@printf "Binary        : %s\n" $(BINARY_NAME)
	@printf "Build path    : %s\n" $(BUILD_DIR)
	@printf "Version       : %s\n" $(VERSION)
	@printf "Build time    : %s\n" $(BUILD_TIME)
	@printf "Go toolchain  : %s\n" "$$($(GO) version)"

clean: ## Remove binaries and Go caches
	@rm -f $(BUILD_DIR)/$(BINARY_NAME)*
	@$(GO) clean -cache -testcache 2>/dev/null || true

utilities: info deps ## Show info and download deps

## Deployment (delegates to deploy/)

docker: ## Build release container image
	@$(MAKE) -C $(DEPLOY_DIR) $@

docker-compose-up: ## Start docker-compose stack
	@$(MAKE) -C $(DEPLOY_DIR) $@

docker-compose-down: ## Stop docker-compose stack
	@$(MAKE) -C $(DEPLOY_DIR) $@

docker-compose-logs: ## Tail docker-compose logs
	@$(MAKE) -C $(DEPLOY_DIR) $@

k8s-namespace: ## Create Kubernetes namespace
	@$(MAKE) -C $(DEPLOY_DIR) $@

k8s-apply: ## Apply Kubernetes manifests
	@$(MAKE) -C $(DEPLOY_DIR) $@

k8s-delete: ## Remove Kubernetes manifests
	@$(MAKE) -C $(DEPLOY_DIR) $@

k8s-logs: ## Follow Kubernetes pod logs
	@$(MAKE) -C $(DEPLOY_DIR) $@

k8s-status: ## Show Kubernetes object status
	@$(MAKE) -C $(DEPLOY_DIR) $@

systemd: ## Install and enable systemd unit
	@$(MAKE) -C $(DEPLOY_DIR) $@

systemd-uninstall: ## Disable and remove systemd unit
	@$(MAKE) -C $(DEPLOY_DIR) $@

systemd-status: ## Show systemd service status
	@$(MAKE) -C $(DEPLOY_DIR) $@

systemd-logs: ## Follow systemd journal logs
	@$(MAKE) -C $(DEPLOY_DIR) $@

deployment: ## Full deployment (docker + k8s + systemd)
	@$(MAKE) -C $(DEPLOY_DIR) $@
