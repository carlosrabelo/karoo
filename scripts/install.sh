#!/bin/bash

# Karoo Installation Script
# This script installs Karoo Stratum Proxy with proper security and configuration

set -euo pipefail

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check Go installation
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.21+ first."
        log_info "Visit: https://golang.org/dl/"
        exit 1
    fi
    
    # Check Go version
    local go_version
    go_version=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
    if ! printf '%s\n' "1.21" "$go_version" | sort -V -C; then
        log_error "Go version $go_version is too old. Please upgrade to Go 1.21+"
        exit 1
    fi
    
    # Check if we're in the right directory
    if [[ ! -f "go.mod" ]] || [[ ! -d "core" ]]; then
        log_error "Please run this script from the Karoo project root directory"
        exit 1
    fi
    
    log_info "Prerequisites check passed"
}

# Set installation paths based on user
setup_paths() {
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
    CORE_DIR="${PROJECT_ROOT}/core"
    BIN_NAME="karoo"
    BUILD_DIR="${PROJECT_ROOT}/bin"
    CONFIG_TEMPLATE="${PROJECT_ROOT}/config/config.example.json"
    
    if [ "$(id -u)" -eq 0 ]; then
        INSTALL_BIN_DIR="/usr/local/bin"
        INSTALL_CONFIG_DIR="/etc/karoo"
        INSTALL_SERVICE_DIR="/etc/systemd/system"
        LOG_DIR="/var/log/karoo"
        DATA_DIR="/var/lib/karoo"
        CREATE_USER=true
    else
        INSTALL_BIN_DIR="${HOME}/.local/bin"
        INSTALL_CONFIG_DIR="${HOME}/.config/karoo"
        LOG_DIR="${HOME}/.local/share/karoo/logs"
        DATA_DIR="${HOME}/.local/share/karoo/data"
        CREATE_USER=false
    fi
}

# Create karoo user if running as root
create_user() {
    if [[ "$CREATE_USER" == true ]] && ! id "karoo" &>/dev/null; then
        log_info "Creating karoo user..."
        useradd -r -s /bin/false -d /var/lib/karoo karoo
        log_info "User karoo created"
    fi
}

# Build the binary
build_binary() {
    log_info "Building Karoo binary..."
    
    mkdir -p "${BUILD_DIR}"
    
    VERSION="$(cd "${CORE_DIR}" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
    BUILD_TIME="$(date +%Y-%m-%dT%H:%M:%S%z)"
    LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"
    
    (
        cd "${CORE_DIR}"
        CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${BUILD_DIR}/${BIN_NAME}" ./cmd/karoo
    )
    
    log_info "Binary built successfully (version: $VERSION)"
}

# Install binary and configuration
install_files() {
    log_info "Installing binary to ${INSTALL_BIN_DIR}..."
    install -d "${INSTALL_BIN_DIR}"
    install -m 755 "${BUILD_DIR}/${BIN_NAME}" "${INSTALL_BIN_DIR}/${BIN_NAME}"
    
    log_info "Setting up configuration in ${INSTALL_CONFIG_DIR}..."
    install -d "${INSTALL_CONFIG_DIR}"
    DEST_CONFIG="${INSTALL_CONFIG_DIR}/config.json"
    if [ -f "${DEST_CONFIG}" ]; then
        log_warn "Existing config preserved at ${DEST_CONFIG}"
    else
        install -m 644 "${CONFIG_TEMPLATE}" "${DEST_CONFIG}"
        log_info "Default configuration installed"
    fi
    
    # Create data and log directories
    if [[ "$CREATE_USER" == true ]]; then
        install -d -o karoo -g karoo "${LOG_DIR}" "${DATA_DIR}"
    else
        install -d "${LOG_DIR}" "${DATA_DIR}"
    fi
}

# Install systemd service (root only)
install_service() {
    if [[ "$CREATE_USER" == true ]]; then
        log_info "Installing systemd service..."
        install -m 644 "${PROJECT_ROOT}/deploy/systemd/karoo.service" "${INSTALL_SERVICE_DIR}/"
        systemctl daemon-reload
        systemctl enable karoo
        log_info "Systemd service installed and enabled"
        log_info "Start with: systemctl start karoo"
        log_info "Check status with: systemctl status karoo"
    fi
}

# Print installation summary
print_summary() {
    log_info "Karoo installation completed successfully!"
    echo
    echo "Installation details:"
    echo "  Binary: ${INSTALL_BIN_DIR}/${BIN_NAME}"
    echo "  Config: ${INSTALL_CONFIG_DIR}/config.json"
    echo "  Logs: ${LOG_DIR}"
    echo "  Data: ${DATA_DIR}"
    echo
    
    if [[ "$CREATE_USER" == false ]]; then
        echo "To run Karoo:"
        echo "  ${INSTALL_BIN_DIR}/${BIN_NAME} -config ${INSTALL_CONFIG_DIR}/config.json"
        echo
        echo "To add to PATH (add to ~/.bashrc or ~/.zshrc):"
        echo "  export PATH=\"${INSTALL_BIN_DIR}:\$PATH\""
    else
        echo "Service management:"
        echo "  Start:   systemctl start karoo"
        echo "  Stop:    systemctl stop karoo"
        echo "  Status:  systemctl status karoo"
        echo "  Logs:    journalctl -u karoo -f"
    fi
    echo
    log_info "Edit ${INSTALL_CONFIG_DIR}/config.json before starting Karoo"
}

# Main installation flow
main() {
    log_info "Starting Karoo installation..."
    
    check_prerequisites
    setup_paths
    create_user
    build_binary
    install_files
    install_service
    print_summary
    
    log_info "Installation completed successfully!"
}

# Run main function
main "$@"
