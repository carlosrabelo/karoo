# Deployment Configurations

This directory contains deployment configurations for different environments.

## Docker

- `Dockerfile` - Production Docker image
- `Dockerfile.dev` - Development Docker image  
- `docker-compose.yml` - Local development setup

### Usage

```bash
# Build and run with docker-compose
cd deploy/docker
docker-compose up -d

# Build image only
docker build -f Dockerfile -t karoo:latest ..
```

## Kubernetes

- `namespace.yaml` - Kubernetes namespace
- `deployment.yaml` - Application deployment
- `service.yaml` - Service configuration
- `ingress.yaml` - Ingress configuration
- `configmap.yaml` - Configuration data

### Usage

```bash
# Apply all configurations
kubectl apply -f k8s/

# Apply individual files
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
```

## Systemd

- `karoo.service` - Systemd service unit file

### Usage

```bash
# Install service
sudo cp deploy/systemd/karoo.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable karoo
sudo systemctl start karoo

# Check status
sudo systemctl status karoo
```