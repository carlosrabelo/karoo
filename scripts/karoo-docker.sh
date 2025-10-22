#!/bin/bash

# Docker helper script for Karoo

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

case "${1:-}" in
    "build")
        echo "Building Karoo Docker image..."
        docker build -t karoo -f "$PROJECT_DIR/docker/Dockerfile" "$PROJECT_DIR"
        ;;
    "run")
        echo "Running Karoo container..."
        docker run -p 8080:8080 --name karoo karoo
        ;;
    "dev")
        echo "Running Karoo in development mode..."
        docker-compose -f "$PROJECT_DIR/docker/docker-compose.yml" up --build
        ;;
    "stop")
        echo "Stopping Karoo container..."
        docker stop karoo || true
        docker rm karoo || true
        ;;
    "clean")
        echo "Cleaning up Docker resources..."
        docker stop karoo || true
        docker rm karoo || true
        docker rmi karoo || true
        ;;
    *)
        echo "Usage: $0 {build|run|dev|stop|clean}"
        echo ""
        echo "Commands:"
        echo "  build  - Build the Docker image"
        echo "  run    - Run the container"
        echo "  dev    - Run in development mode with docker-compose"
        echo "  stop   - Stop and remove the container"
        echo "  clean  - Clean up all Docker resources"
        exit 1
        ;;
esac