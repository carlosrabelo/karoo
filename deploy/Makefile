## Karoo Deployment Module

PROJECT_ROOT        := ..
DOCKER_CONTEXT      := ..
DOCKER_IMAGE        := karoo:latest
DOCKER_DEV_IMAGE    := karoo:dev
K8S_NAMESPACE       := karoo
SYSTEMD_PATH        := /etc/systemd/system/karoo.service
CONFIG_DIR          := /etc/karoo

.DEFAULT_GOAL       := help

.PHONY: help docker docker-dev docker-compose-up docker-compose-down docker-compose-logs \
	k8s-namespace k8s-apply k8s-delete k8s-logs k8s-status systemd systemd-install \
	systemd-uninstall systemd-status systemd-logs clean info quality testing deployment utilities %

help:
	@printf "Karoo Deployment Module\n\n"
	@printf "Docker\n"
	@printf "  %-15s %s\n" "docker"     "Build release image deploy/docker/Dockerfile"
	@printf "  %-15s %s\n" "docker-dev" "Build dev image with tooling"
	@printf "  %-15s %s\n" "docker-compose-up" "Start stack defined in docker-compose"
	@printf "  %-15s %s\n" "docker-compose-down" "Stop docker-compose stack"
	@printf "  %-15s %s\n\n" "docker-compose-logs" "Tail docker-compose logs"
	@printf "Kubernetes\n"
	@printf "  %-15s %s\n" "k8s-namespace" "Create namespace before deploy"
	@printf "  %-15s %s\n" "k8s-apply"     "Apply manifests under deploy/k8s"
	@printf "  %-15s %s\n" "k8s-delete"    "Remove manifests from cluster"
	@printf "  %-15s %s\n" "k8s-logs"      "Tail pod logs by label app=karoo"
	@printf "  %-15s %s\n\n" "k8s-status"   "Show namespace object status"
	@printf "Systemd\n"
	@printf "  %-15s %s\n" "systemd"          "Install unit file and enable service"
	@printf "  %-15s %s\n" "systemd-uninstall" "Disable service and remove unit"
	@printf "  %-15s %s\n" "systemd-status"    "Show systemd service status"
	@printf "  %-15s %s\n\n" "systemd-logs"     "Follow journal logs"
	@printf "Quality\n"
	@printf "  %-15s %s\n" "quality" "Run quality checks for deploy"
	@printf "\n"
	@printf "Testing\n"
	@printf "  %-15s %s\n" "testing" "Run tests for deploy"
	@printf "\n"
	@printf "Deployment\n"
	@printf "  %-15s %s\n" "deployment" "Run all deployment tasks"
	@printf "\n"
	@printf "Utilities\n"
	@printf "  %-15s %s\n" "utilities" "Run utility commands"
	@printf "  %-15s %s\n" "clean" "Prune docker system resources"
	@printf "  %-15s %s\n" "info" "Show deployment configuration summary"

docker:
	@docker build -t $(DOCKER_IMAGE) -f docker/Dockerfile $(DOCKER_CONTEXT)

docker-dev:
	@docker build -t $(DOCKER_DEV_IMAGE) -f docker/Dockerfile.dev $(DOCKER_CONTEXT)

docker-compose-up:
	@cd docker && docker-compose up -d

docker-compose-down:
	@cd docker && docker-compose down

docker-compose-logs:
	@cd docker && docker-compose logs -f

k8s-namespace:
	@kubectl apply -f k8s/namespace.yaml

k8s-apply: k8s-namespace
	@kubectl apply -f k8s/configmap.yaml
	@kubectl apply -f k8s/deployment.yaml
	@kubectl apply -f k8s/service.yaml
	@kubectl apply -f k8s/ingress.yaml

k8s-delete:
	@kubectl delete -f k8s/ingress.yaml --ignore-not-found=true
	@kubectl delete -f k8s/service.yaml --ignore-not-found=true
	@kubectl delete -f k8s/deployment.yaml --ignore-not-found=true
	@kubectl delete -f k8s/configmap.yaml --ignore-not-found=true
	@kubectl delete -f k8s/namespace.yaml --ignore-not-found=true

k8s-logs:
	@kubectl logs -n $(K8S_NAMESPACE) -l app=karoo -f

k8s-status:
	@kubectl get all -n $(K8S_NAMESPACE)

systemd: systemd-install

systemd-install:
	@sudo install -m 644 systemd/karoo.service $(SYSTEMD_PATH)
	@sudo systemctl daemon-reload
	@sudo systemctl enable karoo
	@echo "Use 'sudo systemctl start karoo' to start the service"

systemd-uninstall:
	@sudo systemctl stop karoo || true
	@sudo systemctl disable karoo || true
	@sudo rm -f $(SYSTEMD_PATH)
	@sudo systemctl daemon-reload

systemd-status:
	@sudo systemctl status karoo

systemd-logs:
	@sudo journalctl -u karoo -f

clean:
	@docker system prune -f
	@docker volume prune -f

info:
	@printf "Docker image     : %s\n" $(DOCKER_IMAGE)
	@printf "Docker dev image : %s\n" $(DOCKER_DEV_IMAGE)
	@printf "Namespace        : %s\n" $(K8S_NAMESPACE)
	@printf "Systemd path     : %s\n" $(SYSTEMD_PATH)
	@printf "Config dir       : %s\n" $(CONFIG_DIR)

quality:
	@echo "No quality checks defined for deploy module"

testing:
	@echo "No tests defined for deploy module"

deployment: docker docker-dev k8s-apply systemd

utilities: clean info

%:
	@echo "make: *** Unknown target '$@'. Use 'make help' for available targets."; exit 2
