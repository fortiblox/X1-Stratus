#!/bin/bash
# X1-Stratus Installer
# Lightweight Verification Node for X1 Blockchain
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/fortiblox/X1-Stratus/main/install.sh | bash

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Configuration
STRATUS_VERSION="0.1.0"
INSTALL_DIR="/opt/x1-stratus"
CONFIG_DIR="$HOME/.config/x1-stratus"
DATA_DIR="/mnt/x1-stratus"
BIN_DIR="/usr/local/bin"
SETTINGS_FILE="$CONFIG_DIR/settings.conf"
GO_VERSION="1.22.5"

# Minimum Requirements
MIN_RAM_GB=2
MIN_DISK_GB=50
MIN_CPU_CORES=2

# Logging
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[PASS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[FAIL]${NC} $1"; }

print_banner() {
    echo ""
    echo -e "${MAGENTA}"
    echo "  ╔═══════════════════════════════════════════════════════════╗"
    echo "  ║                                                           ║"
    echo "  ║   ██╗  ██╗ ██╗      ███████╗████████╗██████╗              ║"
    echo "  ║   ╚██╗██╔╝███║      ██╔════╝╚══██╔══╝██╔══██╗             ║"
    echo "  ║    ╚███╔╝ ╚██║█████╗███████╗   ██║   ██████╔╝             ║"
    echo "  ║    ██╔██╗  ██║╚════╝╚════██║   ██║   ██╔══██╗             ║"
    echo "  ║   ██╔╝ ██╗ ██║      ███████║   ██║   ██║  ██║             ║"
    echo "  ║   ╚═╝  ╚═╝ ╚═╝      ╚══════╝   ╚═╝   ╚═╝  ╚═╝             ║"
    echo "  ║                                                           ║"
    echo "  ║           The Lightweight Verifier of X1                  ║"
    echo "  ║                                                           ║"
    echo "  ╚═══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
}

print_step() {
    local step=$1
    local total=$2
    local desc=$3
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}Step ${step}/${total}: ${desc}${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# ═══════════════════════════════════════════════════════════════
# System Checks
# ═══════════════════════════════════════════════════════════════

check_root() {
    if [[ $EUID -eq 0 ]]; then
        log_error "Do not run this script as root. Run as a regular user with sudo access."
        exit 1
    fi

    if ! sudo -v &>/dev/null; then
        log_error "This script requires sudo access."
        exit 1
    fi
}

check_os() {
    if [[ ! -f /etc/os-release ]]; then
        log_error "Cannot detect OS. Only Linux is supported."
        exit 1
    fi

    source /etc/os-release

    case "$ID" in
        ubuntu|debian)
            PKG_MANAGER="apt-get"
            PKG_UPDATE="sudo apt-get update"
            PKG_INSTALL="sudo apt-get install -y"
            ;;
        centos|rhel|fedora|rocky|almalinux)
            PKG_MANAGER="dnf"
            PKG_UPDATE="sudo dnf check-update || true"
            PKG_INSTALL="sudo dnf install -y"
            ;;
        *)
            log_error "Unsupported OS: $ID. Supported: Ubuntu, Debian, CentOS, RHEL, Fedora"
            exit 1
            ;;
    esac

    log_success "Detected OS: $PRETTY_NAME"
}

check_requirements() {
    local errors=0

    # Check RAM
    local total_ram_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    local total_ram_gb=$((total_ram_kb / 1024 / 1024))

    if [[ $total_ram_gb -lt $MIN_RAM_GB ]]; then
        log_error "Insufficient RAM: ${total_ram_gb}GB (minimum: ${MIN_RAM_GB}GB)"
        errors=$((errors + 1))
    else
        log_success "RAM: ${total_ram_gb}GB"
    fi

    # Check CPU cores
    local cpu_cores=$(nproc)
    if [[ $cpu_cores -lt $MIN_CPU_CORES ]]; then
        log_error "Insufficient CPU cores: ${cpu_cores} (minimum: ${MIN_CPU_CORES})"
        errors=$((errors + 1))
    else
        log_success "CPU cores: ${cpu_cores}"
    fi

    # Check disk space
    local available_gb=$(df -BG /mnt 2>/dev/null | tail -1 | awk '{print $4}' | tr -d 'G')
    if [[ -z "$available_gb" ]]; then
        available_gb=$(df -BG / | tail -1 | awk '{print $4}' | tr -d 'G')
    fi

    if [[ $available_gb -lt $MIN_DISK_GB ]]; then
        log_error "Insufficient disk space: ${available_gb}GB (minimum: ${MIN_DISK_GB}GB)"
        errors=$((errors + 1))
    else
        log_success "Disk space: ${available_gb}GB available"
    fi

    if [[ $errors -gt 0 ]]; then
        echo ""
        log_error "System does not meet minimum requirements."
        exit 1
    fi
}

# ═══════════════════════════════════════════════════════════════
# Installation Functions
# ═══════════════════════════════════════════════════════════════

install_dependencies() {
    log_info "Updating package lists..."
    $PKG_UPDATE

    log_info "Installing build dependencies..."
    if [[ "$PKG_MANAGER" == "apt-get" ]]; then
        $PKG_INSTALL build-essential git curl wget pkg-config libssl-dev
    else
        $PKG_INSTALL gcc gcc-c++ make git curl wget openssl-devel
    fi

    log_success "Dependencies installed"
}

install_go() {
    if command -v go &>/dev/null; then
        local current_version=$(go version | awk '{print $3}' | sed 's/go//')
        log_info "Go already installed: $current_version"

        # Check if version is sufficient
        if [[ "$(printf '%s\n' "1.22" "$current_version" | sort -V | head -n1)" == "1.22" ]]; then
            log_success "Go version is sufficient"
            return
        fi
    fi

    log_info "Installing Go ${GO_VERSION}..."

    cd /tmp
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
    rm "go${GO_VERSION}.linux-amd64.tar.gz"

    # Add to PATH
    if ! grep -q '/usr/local/go/bin' ~/.bashrc; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    fi
    export PATH=$PATH:/usr/local/go/bin

    log_success "Go ${GO_VERSION} installed"
}

create_directories() {
    log_info "Creating directories..."

    sudo mkdir -p "$INSTALL_DIR/bin"
    sudo mkdir -p "$DATA_DIR"/{accounts,blockstore}
    mkdir -p "$CONFIG_DIR"

    sudo chown -R $USER:$USER "$DATA_DIR"
    sudo chown -R $USER:$USER "$INSTALL_DIR"

    log_success "Directories created"
}

clone_and_build() {
    log_info "Cloning X1-Stratus repository..."

    cd /tmp
    rm -rf X1-Stratus
    git clone https://github.com/fortiblox/X1-Stratus.git
    cd X1-Stratus

    log_info "Building X1-Stratus (this may take a few minutes)..."

    export PATH=$PATH:/usr/local/go/bin
    go mod tidy
    go build -o stratus ./cmd/stratus

    sudo mv stratus "$INSTALL_DIR/bin/"

    log_success "X1-Stratus built successfully"
}

create_config() {
    log_info "Creating default configuration..."

    cat > "$CONFIG_DIR/config.yaml" << EOF
# X1-Stratus Configuration
# Edit this file or use environment variables

# Geyser gRPC endpoint (required)
# Get access from: https://triton.one, https://helius.dev, or https://quicknode.com
geyser_endpoint: ""
geyser_token: ""
geyser_tls: true

# Data directories
data_dir: "$DATA_DIR"

# Commitment level: processed, confirmed, or finalized
commitment: "confirmed"

# Logging: debug, info, warn, error
log_level: "info"

# Optional: start from a specific slot (0 = latest)
start_slot: 0
EOF

    log_success "Configuration created at $CONFIG_DIR/config.yaml"
}

create_wrapper_script() {
    log_info "Creating wrapper script..."

    sudo tee "$BIN_DIR/x1-stratus" > /dev/null << 'EOF'
#!/bin/bash
# X1-Stratus wrapper script

INSTALL_DIR="/opt/x1-stratus"
CONFIG_DIR="$HOME/.config/x1-stratus"
DATA_DIR="/mnt/x1-stratus"
BINARY="$INSTALL_DIR/bin/stratus"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

case "$1" in
    start)
        if systemctl is-active x1-stratus &>/dev/null; then
            echo -e "${YELLOW}X1-Stratus is already running${NC}"
        else
            sudo systemctl start x1-stratus
            echo -e "${GREEN}X1-Stratus started${NC}"
        fi
        ;;
    stop)
        sudo systemctl stop x1-stratus
        echo -e "${YELLOW}X1-Stratus stopped${NC}"
        ;;
    restart)
        sudo systemctl restart x1-stratus
        echo -e "${GREEN}X1-Stratus restarted${NC}"
        ;;
    status)
        $BINARY status --data-dir "$DATA_DIR"
        ;;
    logs)
        journalctl -u x1-stratus -f
        ;;
    config)
        ${EDITOR:-nano} "$CONFIG_DIR/config.yaml"
        ;;
    version)
        $BINARY version
        ;;
    *)
        echo "X1-Stratus - Lightweight Verification Node"
        echo ""
        echo "Usage: x1-stratus <command>"
        echo ""
        echo "Commands:"
        echo "  start     Start the verification node"
        echo "  stop      Stop the verification node"
        echo "  restart   Restart the verification node"
        echo "  status    Show current sync status"
        echo "  logs      Follow live logs"
        echo "  config    Edit configuration file"
        echo "  version   Show version information"
        ;;
esac
EOF

    sudo chmod +x "$BIN_DIR/x1-stratus"
    log_success "Wrapper script created"
}

create_systemd_service() {
    log_info "Creating systemd service..."

    # Read config for geyser settings
    local geyser_endpoint=""
    local geyser_token=""

    if [[ -f "$CONFIG_DIR/config.yaml" ]]; then
        geyser_endpoint=$(grep 'geyser_endpoint:' "$CONFIG_DIR/config.yaml" | awk '{print $2}' | tr -d '"')
        geyser_token=$(grep 'geyser_token:' "$CONFIG_DIR/config.yaml" | awk '{print $2}' | tr -d '"')
    fi

    sudo tee /etc/systemd/system/x1-stratus.service > /dev/null << EOF
[Unit]
Description=X1-Stratus Lightweight Verification Node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$USER
Environment="PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"
Environment="GEYSER_TOKEN=${geyser_token}"
ExecStart=$INSTALL_DIR/bin/stratus run \\
    --geyser-endpoint "${geyser_endpoint}" \\
    --data-dir "$DATA_DIR" \\
    --commitment confirmed \\
    --log-level info
Restart=on-failure
RestartSec=10
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    log_success "Systemd service created"
}

configure_geyser() {
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}Geyser Configuration${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo "X1-Stratus needs a Geyser gRPC endpoint to receive blocks."
    echo ""
    echo "Providers offering Geyser access:"
    echo "  - Triton One: https://triton.one"
    echo "  - Helius: https://helius.dev"
    echo "  - QuickNode: https://quicknode.com"
    echo ""

    read -p "Enter Geyser endpoint (e.g., grpc.triton.one:443): " geyser_endpoint
    read -p "Enter Geyser token (or press Enter to skip): " geyser_token

    if [[ -n "$geyser_endpoint" ]]; then
        sed -i "s|geyser_endpoint: .*|geyser_endpoint: \"$geyser_endpoint\"|" "$CONFIG_DIR/config.yaml"
    fi

    if [[ -n "$geyser_token" ]]; then
        sed -i "s|geyser_token: .*|geyser_token: \"$geyser_token\"|" "$CONFIG_DIR/config.yaml"
    fi

    # Update systemd service with new values
    create_systemd_service

    log_success "Geyser configuration saved"
}

# ═══════════════════════════════════════════════════════════════
# Main Installation
# ═══════════════════════════════════════════════════════════════

main() {
    print_banner

    echo -e "${BOLD}X1-Stratus Installer v${STRATUS_VERSION}${NC}"
    echo ""
    echo "This will install the X1-Stratus lightweight verification node."
    echo "Unlike X1-Aether or X1-Forge, Stratus uses minimal resources (~2GB RAM)"
    echo "by verifying blocks via Geyser streaming instead of running a full validator."
    echo ""

    read -p "Continue with installation? [Y/n] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]?$ ]]; then
        echo "Installation cancelled."
        exit 0
    fi

    local total_steps=8

    print_step 1 $total_steps "Checking prerequisites"
    check_root
    check_os
    check_requirements

    print_step 2 $total_steps "Installing dependencies"
    install_dependencies

    print_step 3 $total_steps "Installing Go"
    install_go

    print_step 4 $total_steps "Creating directories"
    create_directories

    print_step 5 $total_steps "Building X1-Stratus"
    clone_and_build

    print_step 6 $total_steps "Creating configuration"
    create_config
    create_wrapper_script
    create_systemd_service

    print_step 7 $total_steps "Configuring Geyser endpoint"
    configure_geyser

    print_step 8 $total_steps "Installation complete"

    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║${NC}   ${BOLD}X1-Stratus installed successfully!${NC}                      ${GREEN}║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Quick Start:"
    echo ""
    echo "  1. Edit configuration (if needed):"
    echo "     ${CYAN}x1-stratus config${NC}"
    echo ""
    echo "  2. Start the node:"
    echo "     ${CYAN}x1-stratus start${NC}"
    echo ""
    echo "  3. Monitor logs:"
    echo "     ${CYAN}x1-stratus logs${NC}"
    echo ""
    echo "  4. Check sync status:"
    echo "     ${CYAN}x1-stratus status${NC}"
    echo ""
    echo "File Locations:"
    echo "  Binary:  $INSTALL_DIR/bin/stratus"
    echo "  Config:  $CONFIG_DIR/config.yaml"
    echo "  Data:    $DATA_DIR/"
    echo ""

    read -p "Start X1-Stratus now? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo systemctl start x1-stratus
        echo ""
        log_success "X1-Stratus is now running!"
        echo "Use 'x1-stratus logs' to monitor progress."
    fi
}

main "$@"
