#!/bin/bash

# Karoo Installation Script
# This script installs Karoo Stratum Proxy with proper security and configuration

set -euo pipefail

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly NC='\033[0m' # No Color
readonly MIN_GO_VERSION="1.18"

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
        log_error "Go is not installed. Please install Go ${MIN_GO_VERSION}+ first."
        log_info "Visit: https://golang.org/dl/"
        exit 1
    fi

    # Check Go version
    local go_version
    go_version=$(go version | grep -oE 'go[0-9]+\.[0-9]+(\.[0-9]+)?' | sed 's/go//')
    if [[ -z "$go_version" ]]; then
        log_error "Unable to determine installed Go version."
        exit 1
    fi
    if ! printf '%s\n' "${MIN_GO_VERSION}" "$go_version" | sort -V -C; then
        log_error "Go version $go_version is too old. Please upgrade to Go ${MIN_GO_VERSION}+"
        exit 1
    fi

    # Check if we're in the right directory
    if [[ ! -f "${PROJECT_ROOT}/go.mod" ]]; then
        log_error "Please run this script from the Karoo project root directory"
        exit 1
    fi

    log_info "Prerequisites check passed"
}

# Prefer the invoking user's home when running under sudo.
user_home() {
    if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
        getent passwd "$SUDO_USER" | cut -d: -f6
        return
    fi
    echo "${HOME}"
}

# Set installation paths: SYSTEM=1 or PREFIX=… → system-wide; else ~/.local/bin
setup_paths() {
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
    BIN_NAME="${BINARY_NAME:-karoo}"
    BUILD_DIR="${PROJECT_ROOT}/bin"
    CONFIG_TEMPLATE="${PROJECT_ROOT}/config.example.json"
    UH="$(user_home)"

    if [ -n "${PREFIX:-}" ]; then
        INSTALL_BIN_DIR="${PREFIX%/}/bin"
        INSTALL_CONFIG_DIR="${PREFIX%/}/etc/karoo"
        LOG_DIR="${PREFIX%/}/var/log/karoo"
        DATA_DIR="${PREFIX%/}/var/lib/karoo"
        CREATE_USER=false
    elif [ "${SYSTEM:-0}" = "1" ]; then
        INSTALL_BIN_DIR="/usr/local/bin"
        INSTALL_CONFIG_DIR="/etc/karoo"
        INSTALL_SERVICE_DIR="/etc/systemd/system"
        LOG_DIR="/var/log/karoo"
        DATA_DIR="/var/lib/karoo"
        CREATE_USER=true
    else
        INSTALL_BIN_DIR="${UH}/.local/bin"
        INSTALL_CONFIG_DIR="${UH}/.config/karoo"
        LOG_DIR="${UH}/.local/share/karoo/logs"
        DATA_DIR="${UH}/.local/share/karoo/data"
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

    VERSION="$(cd "${PROJECT_ROOT}" && git describe --tags --always --dirty 2>/dev/null || echo dev)"
    BUILD_TIME="$(date +%Y-%m-%dT%H:%M:%S%z)"
    LDFLAGS="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

    cd "${PROJECT_ROOT}"
    CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags="${LDFLAGS}" -o "${BUILD_DIR}/${BIN_NAME}" ./karoo/cmd/karoo

    log_info "Binary built successfully (version: $VERSION)"
}

# Install binary and configuration
install_files() {
    local src="${BUILD_DIR}/${BIN_NAME}"
    local dest="${INSTALL_BIN_DIR}/${BIN_NAME}"

    if [ ! -x "$src" ]; then
        log_error "Missing binary: $src"
        log_info "hint: run make build first"
        exit 1
    fi

    log_info "Installing binary to ${INSTALL_BIN_DIR}..."
    UH="$(user_home)"
    if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
        case "$INSTALL_BIN_DIR" in
            "$UH" | "$UH"/*)
                sudo -u "$SUDO_USER" install -D -m 755 "$src" "$dest"
                ;;
            *)
                install -D -m 755 "$src" "$dest"
                ;;
        esac
    elif [ -w "$(dirname "$INSTALL_BIN_DIR")" ] 2>/dev/null || [ -w "$INSTALL_BIN_DIR" ] 2>/dev/null; then
        install -D -m 755 "$src" "$dest"
    else
        log_warn "Destination not writable; using sudo for install only..."
        sudo install -D -m 755 "$src" "$dest"
    fi

    log_info "Setting up configuration in ${INSTALL_CONFIG_DIR}..."
    if [ -w "$(dirname "$INSTALL_CONFIG_DIR")" ] 2>/dev/null || [ -w "$INSTALL_CONFIG_DIR" ] 2>/dev/null; then
        install -d "${INSTALL_CONFIG_DIR}"
    else
        sudo install -d "${INSTALL_CONFIG_DIR}"
    fi
    DEST_CONFIG="${INSTALL_CONFIG_DIR}/config.json"
    if [ -f "${DEST_CONFIG}" ]; then
        log_warn "Existing config preserved at ${DEST_CONFIG}"
    else
        if [ -w "${INSTALL_CONFIG_DIR}" ] 2>/dev/null; then
            install -m 644 "${CONFIG_TEMPLATE}" "${DEST_CONFIG}"
        else
            sudo install -m 644 "${CONFIG_TEMPLATE}" "${DEST_CONFIG}"
        fi
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
    setup_paths
    log_info "Starting Karoo installation..."

    check_prerequisites
    create_user
    # Rebuild only when the binary is missing (Makefile already runs build first).
    if [ ! -x "${BUILD_DIR}/${BIN_NAME}" ]; then
        build_binary
    fi
    install_files
    install_service
    print_summary

    log_info "Installation completed successfully!"
}

# Run main function
main "$@"
