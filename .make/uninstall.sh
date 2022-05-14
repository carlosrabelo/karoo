#!/bin/bash

# Karoo Uninstallation Script
# This script removes Karoo Stratum Proxy and its components

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

# Confirmation prompt
confirm() {
    local prompt="$1"
    local default="${2:-n}"
    
    if [[ "$default" == "y" ]]; then
        prompt="$prompt [Y/n] "
    else
        prompt="$prompt [y/N] "
    fi
    
    while true; do
        read -r -p "$prompt" response
        case "$response" in
            [Yy][Ee][Ss]|[Yy])
                return 0
                ;;
            [Nn][Oo]|[Nn]|"")
                return 1
                ;;
            *)
                echo "Please answer yes or no."
                ;;
        esac
    done
}

# Stop and remove systemd service
remove_service() {
    if [ "$(id -u)" -eq 0 ]; then
        log_info "Checking for systemd service..."
        
        if systemctl is-active --quiet karoo 2>/dev/null; then
            log_info "Stopping karoo service..."
            systemctl stop karoo
        fi
        
        if systemctl is-enabled --quiet karoo 2>/dev/null; then
            log_info "Disabling karoo service..."
            systemctl disable karoo
        fi
        
        if [ -f "/etc/systemd/system/karoo.service" ]; then
            log_info "Removing systemd service file..."
            rm -f /etc/systemd/system/karoo.service
            systemctl daemon-reload
        fi
    fi
}

# Remove binary
remove_binary() {
    local binary_paths=(
        "/usr/local/bin/karoo"
        "${HOME}/.local/bin/karoo"
        "/usr/bin/karoo"
    )
    
    local binary_found=false
    
    for path in "${binary_paths[@]}"; do
        if [ -f "$path" ]; then
            log_info "Removing binary: $path"
            rm -f "$path"
            binary_found=true
        fi
    done
    
    if ! $binary_found; then
        log_warn "No Karoo binary found in standard locations"
    fi
}

# Remove configuration and data
remove_config_data() {
    local config_dirs=(
        "/etc/karoo"
        "${HOME}/.config/karoo"
    )
    
    local data_dirs=(
        "/var/lib/karoo"
        "/var/log/karoo"
        "${HOME}/.local/share/karoo"
    )
    
    # Remove configuration
    for dir in "${config_dirs[@]}"; do
        if [ -d "$dir" ]; then
            if confirm "Remove configuration directory $dir?"; then
                log_info "Removing configuration: $dir"
                rm -rf "$dir"
            else
                log_warn "Configuration preserved in $dir"
            fi
        fi
    done
    
    # Remove data and logs
    for dir in "${data_dirs[@]}"; do
        if [ -d "$dir" ]; then
            if confirm "Remove data directory $dir?"; then
                log_info "Removing data: $dir"
                rm -rf "$dir"
            else
                log_warn "Data preserved in $dir"
            fi
        fi
    done
}

# Remove user (root only)
remove_user() {
    if [ "$(id -u)" -eq 0 ] && id "karoo" &>/dev/null; then
        if confirm "Remove karoo system user?"; then
            log_info "Removing karoo user..."
            userdel -r karoo 2>/dev/null || log_warn "Could not remove user home directory"
        else
            log_warn "User karoo preserved"
        fi
    fi
}

# Print uninstallation summary
print_summary() {
    log_info "Karoo uninstallation completed!"
    echo
    echo "Note: The following may require manual cleanup:"
    echo "  - Any custom systemd service files in /etc/systemd/system/"
    echo "  - Any log entries in journald (journalctl -u karoo)"
    echo "  - Any firewall rules for Karoo ports"
    echo "  - Any custom scripts or cron jobs"
    echo
    log_info "Thank you for using Karoo!"
}

# Main uninstallation flow
main() {
    log_info "Starting Karoo uninstallation..."
    
    if ! confirm "This will remove Karoo and all its components. Continue?"; then
        log_info "Uninstallation cancelled."
        exit 0
    fi
    
    remove_service
    remove_binary
    remove_config_data
    remove_user
    print_summary
}

# Run main function
main "$@"