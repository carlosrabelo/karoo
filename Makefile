## Karoo - Stratum V1 Proxy (root orchestrator)

ROOT_DIR        := $(CURDIR)
CORE_DIR        := core
DEPLOY_DIR      := deploy
FORWARD_DIRS    := $(CORE_DIR) $(DEPLOY_DIR)
MAKEFLAGS      += --no-print-directory

.DEFAULT_GOAL   := help

.PHONY: help build run install test lint mod-tidy docker systemd clean info \
	quality testing deployment utilities core-% deploy-% %

help:
	@printf "Karoo - Stratum V1 Proxy\n\n"
	@printf "Build & Install\n"
	@printf "  %-15s %s\n" "build" "Build Go binary via core module"
	@printf "  %-15s %s\n" "install" "Install binary (auto root/user paths)"
	@printf "  %-15s %s\n" "run" "Run proxy using local config.json"
	@printf "  %-15s %s\n" "clean" "Remove build artifacts (core)"
	@printf "\n"
	@printf "Quality\n"
	@printf "  %-15s %s\n" "quality" "Run quality checks via core"
	@printf "  %-15s %s\n" "lint" "Run golangci-lint through core"
	@printf "  %-15s %s\n" "mod-tidy" "Execute go mod tidy and verify"
	@printf "\n"
	@printf "Testing\n"
	@printf "  %-15s %s\n" "testing" "Run test suite via core"
	@printf "  %-15s %s\n" "test" "Execute go test ./... from core"
	@printf "\n"
	@printf "Deployment\n"
	@printf "  %-15s %s\n" "deployment" "Deploy via deploy module"
	@printf "  %-15s %s\n" "docker" "Build container image via deploy"
	@printf "  %-15s %s\n" "systemd" "Install systemd unit via deploy"
	@printf "\n"
	@printf "Utilities\n"
	@printf "  %-15s %s\n" "utilities" "Utility commands via core"
	@printf "  %-15s %s\n" "info" "Show project information from core"
	@printf "  %-15s %s\n" "core-<tgt>" "Forward target to core Makefile"
	@printf "  %-15s %s\n" "deploy-<tgt>" "Forward target to deploy Makefile"

build run install test lint mod-tidy:
	@$(MAKE) -C $(CORE_DIR) $@

docker systemd:
	@$(MAKE) -C $(DEPLOY_DIR) $@

clean:
	@$(MAKE) -C $(CORE_DIR) $@

info:
	@$(MAKE) -C $(CORE_DIR) $@

quality:
	@$(MAKE) -C $(CORE_DIR) $@

testing:
	@$(MAKE) -C $(CORE_DIR) $@

deployment:
	@$(MAKE) -C $(DEPLOY_DIR) $@

utilities:
	@$(MAKE) -C $(CORE_DIR) $@

core-%:
	@$(MAKE) -C $(CORE_DIR) $*

deploy-%:
	@$(MAKE) -C $(DEPLOY_DIR) $*

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
