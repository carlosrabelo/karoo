CORE_DIR := core
DEPLOY_DIR := deploy
FORWARD_DIRS := $(CORE_DIR) $(DEPLOY_DIR)

.DEFAULT_GOAL := help

MAKEFLAGS += --no-print-directory

.PHONY: help build clean core-% deploy-% deployment docker info install lint mod-tidy quality run systemd test testing utilities

build: ## Build Go binary via core module
	@$(MAKE) -C $(CORE_DIR) build

clean: ## Remove build artifacts (core)
	@$(MAKE) -C $(CORE_DIR) clean

core-%: ## Forward target to core Makefile
	@$(MAKE) -C $(CORE_DIR) $*

deploy-%: ## Forward target to deploy Makefile
	@$(MAKE) -C $(DEPLOY_DIR) $*

deployment: ## Deploy via deploy module
	@$(MAKE) -C $(DEPLOY_DIR) deployment

docker: ## Build container image via deploy
	@$(MAKE) -C $(DEPLOY_DIR) docker

help: ## Show available targets
	@echo "KAROO - Stratum V1 Proxy"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*## "} {printf "  %-15s %s\n", $$1, $$2}'
	@echo ""
	@echo "For more targets, run 'make -C core help' or 'make -C deploy help'"

info: ## Show project information from core
	@$(MAKE) -C $(CORE_DIR) info

install: ## Install binary (auto root/user paths)
	@$(MAKE) -C $(CORE_DIR) install

lint: ## Run golangci-lint through core
	@$(MAKE) -C $(CORE_DIR) lint

mod-tidy: ## Execute go mod tidy and verify
	@$(MAKE) -C $(CORE_DIR) mod-tidy

quality: ## Run quality checks via core
	@$(MAKE) -C $(CORE_DIR) quality

run: ## Run proxy using local config.json
	@$(MAKE) -C $(CORE_DIR) run

systemd: ## Install systemd unit via deploy
	@$(MAKE) -C $(DEPLOY_DIR) systemd

test: ## Execute go test ./... from core
	@$(MAKE) -C $(CORE_DIR) test

testing: ## Run test suite via core
	@$(MAKE) -C $(CORE_DIR) testing

utilities: ## Utility commands via core
	@$(MAKE) -C $(CORE_DIR) utilities

define forward_target
set -e; \
for dir in $(FORWARD_DIRS); do \
	if $(MAKE) -C $$dir -n $(1) >/dev/null 2>&1; then \
		$(MAKE) -C $$dir $(1); \
		exit 0; \
	fi; \
done; \
echo "make: *** Unknown target '$(1)'"; exit 2
endef

%:
	@$(call forward_target,$@)
