#!/bin/bash
#
# X1-Stratus Installer
# Lightweight Verification Node for X1 Blockchain
#
# Usage: curl -sSL https://raw.githubusercontent.com/fortiblox/X1-Stratus/main/install.sh | bash
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
WHITE='\033[1;37m'
BOLD='\033[1m'
NC='\033[0m'

# Configuration
REPO_URL="https://github.com/fortiblox/X1-Stratus"
INSTALL_DIR="/opt/x1-stratus"
DATA_DIR="/mnt/x1-stratus"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/root/.config/x1-stratus"
SERVICE_FILE="/etc/systemd/system/x1-stratus.service"
VERSION_FILE="$INSTALL_DIR/.version"
GO_VERSION="1.22.5"

# Default settings
DEFAULT_COMMITMENT="confirmed"
DEFAULT_SLOT_THRESHOLD="50"
DEFAULT_LOG_LEVEL="info"
DEFAULT_AUTO_UPDATE="true"
DEFAULT_REDISCOVER_INTERVAL="5"

# Print banner
print_banner() {
    echo ""
    echo -e "${MAGENTA}"
    echo "  ╔═══════════════════════════════════════════════════════════════╗"
    echo "  ║                                                               ║"
    echo "  ║     ██╗  ██╗ ██╗      ███████╗████████╗██████╗  █████╗       ║"
    echo "  ║     ╚██╗██╔╝███║      ██╔════╝╚══██╔══╝██╔══██╗██╔══██╗      ║"
    echo "  ║      ╚███╔╝ ╚██║█████╗███████╗   ██║   ██████╔╝███████║      ║"
    echo "  ║      ██╔██╗  ██║╚════╝╚════██║   ██║   ██╔══██╗██╔══██║      ║"
    echo "  ║     ██╔╝ ██╗ ██║      ███████║   ██║   ██║  ██║██║  ██║      ║"
    echo "  ║     ╚═╝  ╚═╝ ╚═╝      ╚══════╝   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝      ║"
    echo "  ║                                                               ║"
    echo "  ║           Lightweight Verification Node for X1                ║"
    echo "  ║                                                               ║"
    echo "  ╚═══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
}

# Logging functions
log_info() { echo -e "${BLUE}[*]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; }

print_step() {
    local step=$1
    local total=$2
    local desc=$3
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}Step ${step}/${total}: ${desc}${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        echo "Please run: sudo bash install.sh"
        echo "Or: curl -sSL ... | sudo bash"
        exit 1
    fi
}

# Detect OS and set package manager
detect_os() {
    if [[ ! -f /etc/os-release ]]; then
        log_error "Cannot detect OS. Only Linux is supported."
        exit 1
    fi

    source /etc/os-release

    case "$ID" in
        ubuntu|debian)
            PKG_MANAGER="apt-get"
            PKG_UPDATE="apt-get update -qq"
            PKG_INSTALL="apt-get install -y -qq"
            ;;
        centos|rhel|fedora|rocky|almalinux)
            PKG_MANAGER="dnf"
            PKG_UPDATE="dnf check-update || true"
            PKG_INSTALL="dnf install -y -q"
            ;;
        *)
            log_warn "Unsupported OS: $ID. Attempting to continue..."
            PKG_MANAGER="apt-get"
            PKG_UPDATE="apt-get update -qq"
            PKG_INSTALL="apt-get install -y -qq"
            ;;
    esac

    log_success "Detected OS: $PRETTY_NAME"
}

# Check system requirements
check_requirements() {
    local errors=0

    # Check RAM (minimum 512MB, recommend 1GB)
    local total_ram_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
    local total_ram_mb=$((total_ram_kb / 1024))

    if [[ $total_ram_mb -lt 512 ]]; then
        log_error "Insufficient RAM: ${total_ram_mb}MB (minimum: 512MB)"
        errors=$((errors + 1))
    else
        log_success "RAM: ${total_ram_mb}MB"
    fi

    # Check disk space (minimum 1GB)
    local available_gb=$(df -BG /opt 2>/dev/null | tail -1 | awk '{print $4}' | tr -d 'G')
    if [[ -z "$available_gb" || "$available_gb" == "-" ]]; then
        available_gb=$(df -BG / | tail -1 | awk '{print $4}' | tr -d 'G')
    fi

    if [[ $available_gb -lt 1 ]]; then
        log_error "Insufficient disk space: ${available_gb}GB (minimum: 1GB)"
        errors=$((errors + 1))
    else
        log_success "Disk space: ${available_gb}GB available"
    fi

    # Check network connectivity
    if curl -s --connect-timeout 5 https://rpc.mainnet.x1.xyz > /dev/null 2>&1; then
        log_success "Network connectivity: OK"
    else
        log_warn "Cannot reach X1 network. May work once network is available."
    fi

    if [[ $errors -gt 0 ]]; then
        echo ""
        log_error "System does not meet minimum requirements."
        exit 1
    fi
}

# Install system dependencies
install_dependencies() {
    log_info "Updating package lists..."
    $PKG_UPDATE > /dev/null 2>&1

    log_info "Installing dependencies..."
    if [[ "$PKG_MANAGER" == "apt-get" ]]; then
        $PKG_INSTALL build-essential git curl wget jq > /dev/null 2>&1
    else
        $PKG_INSTALL gcc gcc-c++ make git curl wget jq > /dev/null 2>&1
    fi

    log_success "Dependencies installed"
}

# Install Go
install_go() {
    if command -v go &>/dev/null; then
        local current_version=$(go version | awk '{print $3}' | sed 's/go//')
        log_info "Go already installed: $current_version"

        # Check if version is sufficient (1.21+)
        local major=$(echo "$current_version" | cut -d. -f1)
        local minor=$(echo "$current_version" | cut -d. -f2)
        if [[ $major -ge 1 && $minor -ge 21 ]]; then
            log_success "Go version is sufficient"
            return
        fi
    fi

    log_info "Installing Go ${GO_VERSION}..."

    cd /tmp
    local arch=$(uname -m)
    local go_arch="amd64"
    if [[ "$arch" == "aarch64" ]]; then
        go_arch="arm64"
    fi

    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-${go_arch}.tar.gz"
    rm "go${GO_VERSION}.linux-${go_arch}.tar.gz"

    # Add to PATH for this session
    export PATH=$PATH:/usr/local/go/bin

    # Add to /etc/profile for all users
    if ! grep -q '/usr/local/go/bin' /etc/profile; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    fi

    log_success "Go ${GO_VERSION} installed"
}

# Create directories
create_directories() {
    log_info "Creating directories..."

    mkdir -p "$INSTALL_DIR"/{bin,scripts,backups}
    mkdir -p "$DATA_DIR"
    mkdir -p "$CONFIG_DIR"

    log_success "Directories created"
}

# Clone and build
clone_and_build() {
    log_info "Downloading X1-Stratus..."

    cd /tmp
    rm -rf X1-Stratus

    git clone --depth 1 "$REPO_URL" X1-Stratus > /dev/null 2>&1
    cd X1-Stratus

    log_info "Building X1-Stratus..."

    export PATH=$PATH:/usr/local/go/bin
    go mod tidy > /dev/null 2>&1
    go build -o stratus ./cmd/stratus

    # Install binary
    mv stratus "$INSTALL_DIR/bin/"
    chmod +x "$INSTALL_DIR/bin/stratus"

    # Copy source for updates
    cd /tmp
    rm -rf "$INSTALL_DIR/src"
    mv X1-Stratus "$INSTALL_DIR/src"

    # Get version
    VERSION=$("$INSTALL_DIR/bin/stratus" --version 2>/dev/null | head -1 | awk '{print $2}' || echo "0.1.0")
    echo "$VERSION" > "$VERSION_FILE"

    log_success "Build complete: X1-Stratus $VERSION"
}

# Create configuration
create_config() {
    log_info "Creating configuration..."

    CONFIG_FILE="$CONFIG_DIR/config.env"

    if [[ ! -f "$CONFIG_FILE" ]]; then
        cat > "$CONFIG_FILE" << EOF
# X1-Stratus Configuration
# Generated on $(date)
# Edit this file or use: x1-stratus config

# Data directory for blockstore
DATA_DIR=$DATA_DIR

# Commitment level: processed, confirmed, finalized
# - processed: fastest, less certain
# - confirmed: balanced (recommended)
# - finalized: slowest, most certain
COMMITMENT=$DEFAULT_COMMITMENT

# Slot threshold for endpoint health
# Endpoints more than this many slots behind are marked unhealthy
SLOT_THRESHOLD=$DEFAULT_SLOT_THRESHOLD

# Log level: debug, info, warn, error
LOG_LEVEL=$DEFAULT_LOG_LEVEL

# Auto-update settings
AUTO_UPDATE=$DEFAULT_AUTO_UPDATE

# Re-discovery interval (minutes)
# How often to scan for new validators on the network
REDISCOVER_INTERVAL=$DEFAULT_REDISCOVER_INTERVAL

# RPC server (disabled by default)
ENABLE_RPC=false
RPC_ADDR=:8899

# Custom reference endpoints (comma-separated)
# Leave empty to use defaults: rpc.mainnet.x1.xyz, entrypoint0/1/2.mainnet.x1.xyz
REFERENCE_ENDPOINTS=
EOF
        log_success "Configuration created: $CONFIG_FILE"
    else
        log_warn "Configuration exists, preserving existing settings"
    fi
}

# Install systemd service
install_service() {
    log_info "Installing systemd service..."

    # Load config
    source "$CONFIG_DIR/config.env" 2>/dev/null || true

    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=X1-Stratus Lightweight Verification Node
Documentation=https://github.com/fortiblox/X1-Stratus
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=$INSTALL_DIR/bin/stratus \\
    --data-dir=${DATA_DIR:-/mnt/x1-stratus} \\
    --commitment=${COMMITMENT:-confirmed} \\
    --slot-threshold=${SLOT_THRESHOLD:-50} \\
    --log-level=${LOG_LEVEL:-info}
Restart=always
RestartSec=10
LimitNOFILE=65535

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=x1-stratus

# Security hardening
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    log_success "Systemd service installed"
}

# Install wrapper script
install_wrapper() {
    log_info "Installing x1-stratus command..."

    cat > "$BIN_DIR/x1-stratus" << 'WRAPPER_EOF'
#!/bin/bash
#
# X1-Stratus Management Wrapper
# https://github.com/fortiblox/X1-Stratus
#

INSTALL_DIR="/opt/x1-stratus"
CONFIG_DIR="/root/.config/x1-stratus"
DATA_DIR="/mnt/x1-stratus"
SERVICE_NAME="x1-stratus"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m'

# Load config
[[ -f "$CONFIG_DIR/config.env" ]] && source "$CONFIG_DIR/config.env"

show_help() {
    echo -e "${CYAN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║${NC}     ${WHITE}X1-Stratus - Lightweight Verification Node${NC}               ${CYAN}║${NC}"
    echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Usage: x1-stratus <command> [options]"
    echo ""
    echo -e "${WHITE}Service Commands:${NC}"
    echo "  start           Start the X1-Stratus service"
    echo "  stop            Stop the X1-Stratus service"
    echo "  restart         Restart the X1-Stratus service"
    echo "  status          Show service status and health"
    echo "  catchup         Show sync progress and slot info"
    echo ""
    echo -e "${WHITE}Monitoring Commands:${NC}"
    echo "  logs [lines]    Show recent logs (default: 50 lines)"
    echo "  follow          Follow logs in real-time"
    echo "  health          Show detailed endpoint health"
    echo "  discover        Run validator discovery"
    echo ""
    echo -e "${WHITE}Configuration Commands:${NC}"
    echo "  config          Open interactive configuration menu"
    echo "  update          Check for and apply updates"
    echo "  rebuild         Rebuild binary from source"
    echo "  identity        Show node identity"
    echo "  version         Show version information"
    echo "  uninstall       Uninstall X1-Stratus"
    echo ""
    echo "Examples:"
    echo "  x1-stratus start"
    echo "  x1-stratus logs 100"
    echo "  x1-stratus config"
}

cmd_start() {
    echo -e "${BLUE}[*]${NC} Starting X1-Stratus..."
    systemctl start $SERVICE_NAME
    sleep 3
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo -e "${GREEN}[✓]${NC} X1-Stratus started successfully"
        echo ""
        cmd_status
    else
        echo -e "${RED}[✗]${NC} Failed to start X1-Stratus"
        echo "Check logs: x1-stratus logs"
        exit 1
    fi
}

cmd_stop() {
    echo -e "${BLUE}[*]${NC} Stopping X1-Stratus..."
    systemctl stop $SERVICE_NAME
    echo -e "${GREEN}[✓]${NC} X1-Stratus stopped"
}

cmd_restart() {
    echo -e "${BLUE}[*]${NC} Restarting X1-Stratus..."
    systemctl restart $SERVICE_NAME
    sleep 3
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo -e "${GREEN}[✓]${NC} X1-Stratus restarted successfully"
    else
        echo -e "${RED}[✗]${NC} Failed to restart X1-Stratus"
        exit 1
    fi
}

cmd_status() {
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                    X1-Stratus Status                           ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""

    # Service status
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo -e "Service:     ${GREEN}● Running${NC}"
    else
        echo -e "Service:     ${RED}● Stopped${NC}"
    fi

    # Version
    VERSION=$(cat "$INSTALL_DIR/.version" 2>/dev/null || echo "unknown")
    echo -e "Version:     ${WHITE}$VERSION${NC}"

    # Uptime
    if systemctl is-active --quiet $SERVICE_NAME; then
        UPTIME=$(systemctl show $SERVICE_NAME --property=ActiveEnterTimestamp | cut -d'=' -f2)
        echo -e "Started:     ${WHITE}$UPTIME${NC}"

        # Memory usage
        PID=$(systemctl show $SERVICE_NAME --property=MainPID | cut -d'=' -f2)
        if [[ "$PID" != "0" && -n "$PID" ]]; then
            MEM=$(ps -p $PID -o rss= 2>/dev/null | awk '{printf "%.1f MB", $1/1024}')
            echo -e "Memory:      ${WHITE}$MEM${NC}"
        fi
    fi

    # Config summary
    echo ""
    echo -e "${YELLOW}Configuration:${NC}"
    echo -e "  Commitment:    ${WHITE}${COMMITMENT:-confirmed}${NC}"
    echo -e "  Slot threshold: ${WHITE}${SLOT_THRESHOLD:-50}${NC}"
    echo -e "  Auto-update:   ${WHITE}${AUTO_UPDATE:-true}${NC}"

    # Latest activity
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo ""
        echo -e "${YELLOW}Latest activity:${NC}"
        journalctl -u $SERVICE_NAME -n 5 --no-pager --output=short 2>/dev/null | tail -5 || echo "No logs available"
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

cmd_logs() {
    LINES=${1:-50}
    journalctl -u $SERVICE_NAME -n $LINES --no-pager
}

cmd_follow() {
    echo -e "${CYAN}Following X1-Stratus logs (Ctrl+C to stop)...${NC}"
    echo ""
    journalctl -u $SERVICE_NAME -f
}

cmd_config() {
    if [[ -f "$BIN_DIR/x1-stratus-config" ]]; then
        "$BIN_DIR/x1-stratus-config"
    else
        echo -e "${YELLOW}[!]${NC} Interactive config tool not found"
        echo "Edit configuration manually: $CONFIG_DIR/config.env"
        echo ""
        read -p "Open in editor? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            ${EDITOR:-nano} "$CONFIG_DIR/config.env"
        fi
    fi
}

cmd_update() {
    echo -e "${BLUE}[*]${NC} Checking for updates..."

    cd "$INSTALL_DIR/src" 2>/dev/null || {
        echo -e "${RED}[✗]${NC} Source directory not found"
        exit 1
    }

    # Fetch latest
    git fetch origin 2>/dev/null

    LOCAL=$(git rev-parse HEAD 2>/dev/null)
    REMOTE=$(git rev-parse origin/main 2>/dev/null)

    if [[ "$LOCAL" == "$REMOTE" ]]; then
        echo -e "${GREEN}[✓]${NC} Already up to date"
        VERSION=$(cat "$INSTALL_DIR/.version" 2>/dev/null || echo "unknown")
        echo "Current version: $VERSION"
        return 0
    fi

    echo -e "${YELLOW}[!]${NC} Update available"
    echo ""

    # Show changes
    echo "Recent changes:"
    git log --oneline HEAD..origin/main | head -10
    echo ""

    read -p "Apply update? [y/N] " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Backup current binary
        BACKUP_DIR="$INSTALL_DIR/backups/$(date +%Y%m%d_%H%M%S)"
        mkdir -p "$BACKUP_DIR"
        cp "$INSTALL_DIR/bin/stratus" "$BACKUP_DIR/" 2>/dev/null || true
        echo -e "${BLUE}[*]${NC} Backup saved to: $BACKUP_DIR"

        # Stop service
        echo -e "${BLUE}[*]${NC} Stopping service..."
        systemctl stop $SERVICE_NAME 2>/dev/null || true

        # Update source
        echo -e "${BLUE}[*]${NC} Downloading update..."
        git reset --hard origin/main

        # Rebuild
        echo -e "${BLUE}[*]${NC} Building..."
        export PATH=$PATH:/usr/local/go/bin
        go build -o "$INSTALL_DIR/bin/stratus" ./cmd/stratus

        if [[ $? -eq 0 ]]; then
            # Update version
            VERSION=$("$INSTALL_DIR/bin/stratus" --version 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown")
            echo "$VERSION" > "$INSTALL_DIR/.version"

            # Restart service
            echo -e "${BLUE}[*]${NC} Restarting service..."
            systemctl start $SERVICE_NAME

            echo ""
            echo -e "${GREEN}[✓]${NC} Updated to $VERSION"
        else
            echo -e "${RED}[✗]${NC} Build failed, rolling back..."
            cp "$BACKUP_DIR/stratus" "$INSTALL_DIR/bin/"
            systemctl start $SERVICE_NAME
            echo -e "${YELLOW}[!]${NC} Rolled back to previous version"
        fi
    else
        echo "Update cancelled"
    fi
}

cmd_discover() {
    echo -e "${BLUE}[*]${NC} Running validator discovery..."
    echo ""

    # Query getClusterNodes
    RESULT=$(curl -s -X POST -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"getClusterNodes"}' \
        https://rpc.mainnet.x1.xyz 2>/dev/null)

    if [[ -z "$RESULT" || "$RESULT" == *"error"* ]]; then
        echo -e "${RED}[✗]${NC} Failed to connect to X1 network"
        exit 1
    fi

    # Parse results
    TOTAL=$(echo "$RESULT" | jq '.result | length' 2>/dev/null || echo "0")
    RPC_NODES=$(echo "$RESULT" | jq '[.result[] | select(.rpc != null)] | length' 2>/dev/null || echo "0")
    GOSSIP_NODES=$(echo "$RESULT" | jq '[.result[] | select(.gossip != null)] | length' 2>/dev/null || echo "0")

    echo -e "${GREEN}[✓]${NC} Discovery complete"
    echo ""
    echo -e "${WHITE}Network Statistics:${NC}"
    echo -e "  Total validators:     ${CYAN}$TOTAL${NC}"
    echo -e "  With RPC endpoints:   ${CYAN}$RPC_NODES${NC}"
    echo -e "  With gossip:          ${CYAN}$GOSSIP_NODES${NC}"
    echo ""

    # Show sample endpoints
    echo -e "${YELLOW}Sample RPC endpoints:${NC}"
    echo "$RESULT" | jq -r '.result[] | select(.rpc != null) | "  http://\(.rpc)"' 2>/dev/null | head -5
    echo ""

    # Get current slot
    SLOT=$(curl -s -X POST -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"getSlot"}' \
        https://rpc.mainnet.x1.xyz 2>/dev/null | jq '.result' 2>/dev/null)

    if [[ -n "$SLOT" && "$SLOT" != "null" ]]; then
        echo -e "${WHITE}Current network slot:${NC} ${CYAN}$SLOT${NC}"
    fi
}

cmd_health() {
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                  Endpoint Health Check                         ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""

    ENDPOINTS=(
        "https://rpc.mainnet.x1.xyz"
        "https://entrypoint0.mainnet.x1.xyz"
        "https://entrypoint1.mainnet.x1.xyz"
        "https://entrypoint2.mainnet.x1.xyz"
    )

    echo -e "${YELLOW}Reference Endpoints:${NC}"
    HIGHEST_SLOT=0

    for ep in "${ENDPOINTS[@]}"; do
        START=$(date +%s%N)
        SLOT=$(curl -s --connect-timeout 5 -X POST -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","id":1,"method":"getSlot"}' \
            "$ep" 2>/dev/null | jq '.result' 2>/dev/null)
        END=$(date +%s%N)

        LATENCY=$(( (END - START) / 1000000 ))

        if [[ -n "$SLOT" && "$SLOT" != "null" ]]; then
            echo -e "  ${GREEN}●${NC} $ep"
            echo -e "    Slot: $SLOT | Latency: ${LATENCY}ms"
            if [[ $SLOT -gt $HIGHEST_SLOT ]]; then
                HIGHEST_SLOT=$SLOT
            fi
        else
            echo -e "  ${RED}●${NC} $ep (unreachable)"
        fi
    done

    echo ""
    echo -e "${WHITE}Reference slot:${NC} $HIGHEST_SLOT"
    echo -e "${WHITE}Slot threshold:${NC} ${SLOT_THRESHOLD:-50}"
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

cmd_catchup() {
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                    Sync Progress                               ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""

    # Get network slot
    NETWORK_SLOT=$(curl -s -X POST -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"getSlot"}' \
        https://rpc.mainnet.x1.xyz 2>/dev/null | jq '.result' 2>/dev/null)

    if [[ -z "$NETWORK_SLOT" || "$NETWORK_SLOT" == "null" ]]; then
        echo -e "${RED}[✗]${NC} Cannot reach X1 network"
        return 1
    fi

    # Get local slot from logs (last processed slot)
    LOCAL_SLOT=$(journalctl -u $SERVICE_NAME -n 100 --no-pager 2>/dev/null | \
        grep -oP 'slot[=:]\s*\K[0-9]+' | tail -1)

    if [[ -z "$LOCAL_SLOT" ]]; then
        LOCAL_SLOT="unknown"
    fi

    echo -e "${WHITE}Network Status:${NC}"
    echo -e "  Network slot:    ${CYAN}$NETWORK_SLOT${NC}"
    echo -e "  Local slot:      ${CYAN}$LOCAL_SLOT${NC}"

    if [[ "$LOCAL_SLOT" != "unknown" ]]; then
        BEHIND=$((NETWORK_SLOT - LOCAL_SLOT))
        if [[ $BEHIND -lt 0 ]]; then
            BEHIND=0
        fi

        if [[ $BEHIND -eq 0 ]]; then
            echo -e "  Status:          ${GREEN}Synced${NC}"
        elif [[ $BEHIND -lt 50 ]]; then
            echo -e "  Status:          ${GREEN}Nearly synced ($BEHIND slots behind)${NC}"
        elif [[ $BEHIND -lt 1000 ]]; then
            echo -e "  Status:          ${YELLOW}Catching up ($BEHIND slots behind)${NC}"
        else
            echo -e "  Status:          ${RED}Far behind ($BEHIND slots)${NC}"
        fi
    fi

    # Get epoch info
    EPOCH_INFO=$(curl -s -X POST -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"getEpochInfo"}' \
        https://rpc.mainnet.x1.xyz 2>/dev/null)

    if [[ -n "$EPOCH_INFO" ]]; then
        EPOCH=$(echo "$EPOCH_INFO" | jq '.result.epoch' 2>/dev/null)
        SLOT_INDEX=$(echo "$EPOCH_INFO" | jq '.result.slotIndex' 2>/dev/null)
        SLOTS_IN_EPOCH=$(echo "$EPOCH_INFO" | jq '.result.slotsInEpoch' 2>/dev/null)

        if [[ -n "$EPOCH" && "$EPOCH" != "null" ]]; then
            echo ""
            echo -e "${WHITE}Epoch Info:${NC}"
            echo -e "  Current epoch:   ${CYAN}$EPOCH${NC}"
            if [[ -n "$SLOT_INDEX" && -n "$SLOTS_IN_EPOCH" ]]; then
                PROGRESS=$((SLOT_INDEX * 100 / SLOTS_IN_EPOCH))
                echo -e "  Epoch progress:  ${CYAN}$PROGRESS%${NC} ($SLOT_INDEX / $SLOTS_IN_EPOCH slots)"
            fi
        fi
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

cmd_rebuild() {
    echo -e "${BLUE}[*]${NC} Rebuilding X1-Stratus from source..."

    if [[ ! -d "$INSTALL_DIR/src" ]]; then
        echo -e "${RED}[✗]${NC} Source directory not found"
        echo "Run: x1-stratus update to download latest source"
        exit 1
    fi

    cd "$INSTALL_DIR/src"

    # Backup current binary
    BACKUP_DIR="$INSTALL_DIR/backups/$(date +%Y%m%d_%H%M%S)"
    mkdir -p "$BACKUP_DIR"
    cp "$INSTALL_DIR/bin/stratus" "$BACKUP_DIR/" 2>/dev/null || true
    echo -e "${BLUE}[*]${NC} Backup saved to: $BACKUP_DIR"

    # Stop service
    WAS_RUNNING=false
    if systemctl is-active --quiet $SERVICE_NAME; then
        WAS_RUNNING=true
        echo -e "${BLUE}[*]${NC} Stopping service..."
        systemctl stop $SERVICE_NAME
    fi

    # Rebuild
    echo -e "${BLUE}[*]${NC} Compiling..."
    export PATH=$PATH:/usr/local/go/bin
    go build -o "$INSTALL_DIR/bin/stratus" ./cmd/stratus

    if [[ $? -eq 0 ]]; then
        VERSION=$("$INSTALL_DIR/bin/stratus" --version 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown")
        echo "$VERSION" > "$INSTALL_DIR/.version"
        echo -e "${GREEN}[✓]${NC} Build complete: $VERSION"

        if [[ "$WAS_RUNNING" == "true" ]]; then
            echo -e "${BLUE}[*]${NC} Restarting service..."
            systemctl start $SERVICE_NAME
            echo -e "${GREEN}[✓]${NC} Service restarted"
        fi
    else
        echo -e "${RED}[✗]${NC} Build failed, restoring backup..."
        cp "$BACKUP_DIR/stratus" "$INSTALL_DIR/bin/"

        if [[ "$WAS_RUNNING" == "true" ]]; then
            systemctl start $SERVICE_NAME
        fi
        exit 1
    fi
}

cmd_identity() {
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}                    Node Identity                               ${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""

    # Check for identity file
    IDENTITY_FILE="$CONFIG_DIR/identity.json"

    if [[ -f "$IDENTITY_FILE" ]]; then
        echo -e "${WHITE}Identity File:${NC} $IDENTITY_FILE"

        # Try to extract pubkey
        PUBKEY=$(jq -r '.pubkey // empty' "$IDENTITY_FILE" 2>/dev/null)
        if [[ -n "$PUBKEY" ]]; then
            echo -e "${WHITE}Public Key:${NC} $PUBKEY"
        fi
    else
        echo -e "${YELLOW}[!]${NC} No identity configured"
        echo ""
        echo "To create an identity, run: x1-stratus config"
        echo "Then select 'Identity Management'"
    fi

    # Show node branding if configured
    if [[ -n "${NODE_NAME:-}" ]]; then
        echo ""
        echo -e "${WHITE}Node Branding:${NC}"
        echo -e "  Name:    ${CYAN}${NODE_NAME:-not set}${NC}"
        echo -e "  Website: ${CYAN}${NODE_WEBSITE:-not set}${NC}"
        echo -e "  Icon:    ${CYAN}${NODE_ICON:-not set}${NC}"
    fi

    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
}

cmd_version() {
    VERSION=$(cat "$INSTALL_DIR/.version" 2>/dev/null || echo "unknown")
    COMMIT="unknown"
    if [[ -d "$INSTALL_DIR/src/.git" ]]; then
        COMMIT=$(cd "$INSTALL_DIR/src" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    fi
    echo "X1-Stratus $VERSION ($COMMIT)"
}

cmd_uninstall() {
    echo -e "${YELLOW}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║              Uninstall X1-Stratus                             ║${NC}"
    echo -e "${YELLOW}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "This will remove:"
    echo "  - X1-Stratus binary and source ($INSTALL_DIR)"
    echo "  - Systemd service"
    echo "  - Configuration files ($CONFIG_DIR)"
    echo "  - Wrapper commands"
    echo ""
    echo -e "${RED}Data directory ($DATA_DIR) will NOT be removed.${NC}"
    echo ""

    read -p "Are you sure you want to uninstall? [y/N] " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo -e "${BLUE}[*]${NC} Stopping service..."
        systemctl stop $SERVICE_NAME 2>/dev/null || true
        systemctl disable $SERVICE_NAME 2>/dev/null || true

        echo -e "${BLUE}[*]${NC} Removing files..."
        rm -f /etc/systemd/system/x1-stratus.service
        rm -f /usr/local/bin/x1-stratus
        rm -f /usr/local/bin/x1-stratus-config
        rm -rf "$INSTALL_DIR"
        rm -rf "$CONFIG_DIR"

        # Remove cron job
        crontab -l 2>/dev/null | grep -v "x1-stratus" | crontab - 2>/dev/null || true

        systemctl daemon-reload

        echo ""
        echo -e "${GREEN}[✓]${NC} X1-Stratus uninstalled"
        echo ""
        echo "Data directory preserved at: $DATA_DIR"
        echo "To remove data: rm -rf $DATA_DIR"
    else
        echo "Uninstall cancelled"
    fi
}

# Main command router
case "${1:-help}" in
    start)      cmd_start ;;
    stop)       cmd_stop ;;
    restart)    cmd_restart ;;
    status)     cmd_status ;;
    catchup)    cmd_catchup ;;
    logs)       cmd_logs "$2" ;;
    follow)     cmd_follow ;;
    config)     cmd_config ;;
    update)     cmd_update ;;
    rebuild)    cmd_rebuild ;;
    identity)   cmd_identity ;;
    discover)   cmd_discover ;;
    health)     cmd_health ;;
    version)    cmd_version ;;
    uninstall)  cmd_uninstall ;;
    help|--help|-h) show_help ;;
    *)
        echo -e "${RED}Unknown command: $1${NC}"
        echo ""
        show_help
        exit 1
        ;;
esac
WRAPPER_EOF

    chmod +x "$BIN_DIR/x1-stratus"
    log_success "x1-stratus command installed"
}

# Install config tool
install_config_tool() {
    log_info "Installing configuration tool..."

    cat > "$BIN_DIR/x1-stratus-config" << 'CONFIG_EOF'
#!/bin/bash
#
# X1-Stratus Interactive Configuration
#

CONFIG_DIR="/root/.config/x1-stratus"
CONFIG_FILE="$CONFIG_DIR/config.env"
SERVICE_NAME="x1-stratus"
INSTALL_DIR="/opt/x1-stratus"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m'

# Load current config
load_config() {
    # Set defaults
    DATA_DIR="/mnt/x1-stratus"
    COMMITMENT="confirmed"
    SLOT_THRESHOLD="50"
    LOG_LEVEL="info"
    AUTO_UPDATE="true"
    REDISCOVER_INTERVAL="5"
    ENABLE_RPC="false"
    RPC_ADDR=":8899"
    REFERENCE_ENDPOINTS=""

    # Node branding defaults
    NODE_NAME=""
    NODE_WEBSITE=""
    NODE_ICON=""

    [[ -f "$CONFIG_FILE" ]] && source "$CONFIG_FILE"
}

# Save config
save_config() {
    mkdir -p "$CONFIG_DIR"
    cat > "$CONFIG_FILE" << EOF
# X1-Stratus Configuration
# Last modified: $(date)

# Data directory for blockstore
DATA_DIR=$DATA_DIR

# Commitment level: processed, confirmed, finalized
COMMITMENT=$COMMITMENT

# Slot threshold for endpoint health
SLOT_THRESHOLD=$SLOT_THRESHOLD

# Log level: debug, info, warn, error
LOG_LEVEL=$LOG_LEVEL

# Auto-update settings
AUTO_UPDATE=$AUTO_UPDATE

# Re-discovery interval (minutes)
REDISCOVER_INTERVAL=$REDISCOVER_INTERVAL

# RPC server
ENABLE_RPC=$ENABLE_RPC
RPC_ADDR=$RPC_ADDR

# Custom reference endpoints (comma-separated)
REFERENCE_ENDPOINTS=$REFERENCE_ENDPOINTS

# Node branding (published on-chain)
NODE_NAME=$NODE_NAME
NODE_WEBSITE=$NODE_WEBSITE
NODE_ICON=$NODE_ICON
EOF
}

# Update systemd service
update_service() {
    cat > /etc/systemd/system/x1-stratus.service << EOF
[Unit]
Description=X1-Stratus Lightweight Verification Node
Documentation=https://github.com/fortiblox/X1-Stratus
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=$INSTALL_DIR/bin/stratus \\
    --data-dir=$DATA_DIR \\
    --commitment=$COMMITMENT \\
    --slot-threshold=$SLOT_THRESHOLD \\
    --log-level=$LOG_LEVEL
Restart=always
RestartSec=10
LimitNOFILE=65535

StandardOutput=journal
StandardError=journal
SyslogIdentifier=x1-stratus

NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

show_header() {
    clear
    echo -e "${CYAN}"
    echo "╔═══════════════════════════════════════════════════════════════╗"
    echo "║            X1-Stratus Configuration                           ║"
    echo "╚═══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

show_main_menu() {
    show_header

    # Status line
    echo -e "${WHITE}Service Status:${NC}"
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo -e "  ${GREEN}● Running${NC}"
        AUTOSTART=$(systemctl is-enabled $SERVICE_NAME 2>/dev/null)
        if [[ "$AUTOSTART" == "enabled" ]]; then
            echo -e "  Auto-start: ${GREEN}enabled${NC}"
        else
            echo -e "  Auto-start: ${YELLOW}disabled${NC}"
        fi
    else
        echo -e "  ${RED}● Stopped${NC}"
    fi
    echo ""

    echo -e "${WHITE}Configuration Menu:${NC}"
    echo ""
    echo "  1) Identity Management     (view, import, generate)"
    echo "  2) Node Branding           (name, website, icon)"
    echo "  3) Network Settings        (commitment, endpoints)"
    echo "  4) Performance Settings    (thresholds, intervals)"
    echo "  5) Logging Settings        (log level)"
    echo "  6) Auto-Start Settings     (toggle startup on boot)"
    echo "  7) Auto-Update Settings    (enable/disable)"
    echo "  8) RPC Server Settings     (enable/disable, port)"
    echo ""
    echo "  9) Rebuild Binary          (recompile from source)"
    echo ""
    echo "  v) View Current Config"
    echo "  a) Apply Changes & Restart"
    echo "  r) Reset to Defaults"
    echo ""
    echo "  0) Exit"
    echo ""
    echo -n "Select option: "
}

menu_identity() {
    show_header
    echo -e "${WHITE}Identity Management${NC}"
    echo ""

    IDENTITY_FILE="$CONFIG_DIR/identity.json"

    if [[ -f "$IDENTITY_FILE" ]]; then
        PUBKEY=$(jq -r '.pubkey // "unknown"' "$IDENTITY_FILE" 2>/dev/null)
        echo -e "Current Identity: ${CYAN}$PUBKEY${NC}"
    else
        echo -e "Current Identity: ${YELLOW}Not configured${NC}"
    fi
    echo ""

    echo "Options:"
    echo "  1) View current identity"
    echo "  2) Generate new identity"
    echo "  3) Import identity from file"
    echo "  4) Export identity"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            if [[ -f "$IDENTITY_FILE" ]]; then
                echo ""
                echo -e "${WHITE}Identity Details:${NC}"
                cat "$IDENTITY_FILE" | jq . 2>/dev/null || cat "$IDENTITY_FILE"
            else
                echo -e "${YELLOW}No identity configured${NC}"
            fi
            echo ""
            read -p "Press Enter to continue..."
            ;;
        2)
            echo ""
            echo -e "${YELLOW}Generating new identity...${NC}"

            # Generate a simple identity (in production, use proper key generation)
            TIMESTAMP=$(date +%s)
            RANDOM_BYTES=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 44)

            cat > "$IDENTITY_FILE" << EOF
{
    "pubkey": "$RANDOM_BYTES",
    "created": "$TIMESTAMP",
    "type": "stratus-node"
}
EOF
            echo -e "${GREEN}[✓]${NC} New identity generated"
            echo -e "Public key: ${CYAN}$RANDOM_BYTES${NC}"
            echo ""
            read -p "Press Enter to continue..."
            ;;
        3)
            echo ""
            echo "Enter path to identity file:"
            echo -n "> "
            read import_path
            if [[ -f "$import_path" ]]; then
                cp "$import_path" "$IDENTITY_FILE"
                echo -e "${GREEN}[✓]${NC} Identity imported"
            else
                echo -e "${RED}[✗]${NC} File not found: $import_path"
            fi
            sleep 2
            ;;
        4)
            if [[ -f "$IDENTITY_FILE" ]]; then
                EXPORT_PATH="/tmp/x1-stratus-identity-$(date +%Y%m%d).json"
                cp "$IDENTITY_FILE" "$EXPORT_PATH"
                echo -e "${GREEN}[✓]${NC} Identity exported to: $EXPORT_PATH"
            else
                echo -e "${RED}[✗]${NC} No identity to export"
            fi
            sleep 2
            ;;
    esac
}

menu_branding() {
    show_header
    echo -e "${WHITE}Node Branding${NC}"
    echo ""
    echo "Configure how your node appears on the network."
    echo ""
    echo "Current values:"
    echo -e "  Node Name:    ${CYAN}${NODE_NAME:-not set}${NC}"
    echo -e "  Website:      ${CYAN}${NODE_WEBSITE:-not set}${NC}"
    echo -e "  Icon URL:     ${CYAN}${NODE_ICON:-not set}${NC}"
    echo ""

    echo "Options:"
    echo "  1) Set Node Name"
    echo "  2) Set Website URL"
    echo "  3) Set Icon URL"
    echo "  4) Clear all branding"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            echo ""
            echo "Enter node name (e.g., 'My X1 Node'):"
            echo -n "> "
            read NODE_NAME
            echo -e "${GREEN}Node name set to: $NODE_NAME${NC}"
            sleep 1
            ;;
        2)
            echo ""
            echo "Enter website URL (e.g., 'https://mynode.example.com'):"
            echo -n "> "
            read NODE_WEBSITE
            echo -e "${GREEN}Website set to: $NODE_WEBSITE${NC}"
            sleep 1
            ;;
        3)
            echo ""
            echo "Enter icon URL (direct link to PNG/JPG image):"
            echo -n "> "
            read NODE_ICON
            echo -e "${GREEN}Icon URL set to: $NODE_ICON${NC}"
            sleep 1
            ;;
        4)
            NODE_NAME=""
            NODE_WEBSITE=""
            NODE_ICON=""
            echo -e "${GREEN}Branding cleared${NC}"
            sleep 1
            ;;
    esac
}

menu_autostart() {
    show_header
    echo -e "${WHITE}Auto-Start Settings${NC}"
    echo ""

    AUTOSTART=$(systemctl is-enabled $SERVICE_NAME 2>/dev/null)
    if [[ "$AUTOSTART" == "enabled" ]]; then
        echo -e "Current status: ${GREEN}Enabled${NC}"
        echo "X1-Stratus will start automatically on system boot."
    else
        echo -e "Current status: ${YELLOW}Disabled${NC}"
        echo "X1-Stratus will NOT start automatically on system boot."
    fi
    echo ""

    echo "Options:"
    echo "  1) Enable auto-start"
    echo "  2) Disable auto-start"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            systemctl enable $SERVICE_NAME 2>/dev/null
            echo -e "${GREEN}[✓]${NC} Auto-start enabled"
            sleep 1
            ;;
        2)
            systemctl disable $SERVICE_NAME 2>/dev/null
            echo -e "${YELLOW}[!]${NC} Auto-start disabled"
            sleep 1
            ;;
    esac
}

menu_rebuild() {
    show_header
    echo -e "${WHITE}Rebuild Binary${NC}"
    echo ""
    echo "This will recompile X1-Stratus from the latest source code."
    echo "Useful after manual code changes or to ensure a clean build."
    echo ""

    if [[ -d "$INSTALL_DIR/src" ]]; then
        CURRENT_COMMIT=$(cd "$INSTALL_DIR/src" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        echo -e "Source directory: ${CYAN}$INSTALL_DIR/src${NC}"
        echo -e "Current commit:   ${CYAN}$CURRENT_COMMIT${NC}"
    else
        echo -e "${YELLOW}[!]${NC} Source directory not found"
    fi
    echo ""

    read -p "Rebuild now? [y/N] " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo ""
        x1-stratus rebuild
        echo ""
        read -p "Press Enter to continue..."
    fi
}

menu_network() {
    show_header
    echo -e "${WHITE}Network Settings${NC}"
    echo ""
    echo "Current values:"
    echo -e "  Commitment:          ${CYAN}$COMMITMENT${NC}"
    echo -e "  Reference Endpoints: ${CYAN}${REFERENCE_ENDPOINTS:-default}${NC}"
    echo ""
    echo "Options:"
    echo "  1) Set Commitment Level"
    echo "  2) Set Custom Reference Endpoints"
    echo "  3) Reset to Default Endpoints"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            echo ""
            echo "Commitment levels:"
            echo "  1) processed  - Fastest, block seen by node"
            echo "  2) confirmed  - Supermajority confirmed (recommended)"
            echo "  3) finalized  - Fully finalized, slowest"
            echo ""
            echo -n "Select [1-3]: "
            read c
            case $c in
                1) COMMITMENT="processed" ;;
                2) COMMITMENT="confirmed" ;;
                3) COMMITMENT="finalized" ;;
            esac
            echo -e "${GREEN}Commitment set to: $COMMITMENT${NC}"
            sleep 1
            ;;
        2)
            echo ""
            echo "Enter comma-separated RPC endpoints:"
            echo "Example: https://rpc1.example.com,https://rpc2.example.com"
            echo ""
            echo -n "> "
            read REFERENCE_ENDPOINTS
            echo -e "${GREEN}Custom endpoints set${NC}"
            sleep 1
            ;;
        3)
            REFERENCE_ENDPOINTS=""
            echo -e "${GREEN}Reset to default endpoints${NC}"
            sleep 1
            ;;
    esac
}

menu_performance() {
    show_header
    echo -e "${WHITE}Performance Settings${NC}"
    echo ""
    echo "Current values:"
    echo -e "  Slot Threshold:        ${CYAN}$SLOT_THRESHOLD${NC} slots"
    echo -e "  Re-discovery Interval: ${CYAN}$REDISCOVER_INTERVAL${NC} minutes"
    echo ""
    echo "Options:"
    echo "  1) Set Slot Threshold"
    echo "  2) Set Re-discovery Interval"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            echo ""
            echo "Slot threshold determines how many slots behind an endpoint"
            echo "can be before being marked unhealthy."
            echo ""
            echo "Lower = stricter (fewer endpoints), Higher = more tolerant"
            echo "Recommended: 50"
            echo ""
            echo -n "Enter threshold [10-500]: "
            read val
            if [[ "$val" =~ ^[0-9]+$ ]] && [[ $val -ge 10 ]] && [[ $val -le 500 ]]; then
                SLOT_THRESHOLD=$val
                echo -e "${GREEN}Slot threshold set to: $val${NC}"
            else
                echo -e "${RED}Invalid value${NC}"
            fi
            sleep 1
            ;;
        2)
            echo ""
            echo "Re-discovery interval determines how often to scan"
            echo "for new validators on the network."
            echo ""
            echo "Recommended: 5 minutes"
            echo ""
            echo -n "Enter interval [1-60] minutes: "
            read val
            if [[ "$val" =~ ^[0-9]+$ ]] && [[ $val -ge 1 ]] && [[ $val -le 60 ]]; then
                REDISCOVER_INTERVAL=$val
                echo -e "${GREEN}Interval set to: $val minutes${NC}"
            else
                echo -e "${RED}Invalid value${NC}"
            fi
            sleep 1
            ;;
    esac
}

menu_logging() {
    show_header
    echo -e "${WHITE}Logging Settings${NC}"
    echo ""
    echo -e "Current log level: ${CYAN}$LOG_LEVEL${NC}"
    echo ""
    echo "Options:"
    echo "  1) debug  - Verbose debugging information"
    echo "  2) info   - General operational messages (recommended)"
    echo "  3) warn   - Warnings only"
    echo "  4) error  - Errors only"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1) LOG_LEVEL="debug"; echo -e "${GREEN}Log level set to: debug${NC}"; sleep 1 ;;
        2) LOG_LEVEL="info"; echo -e "${GREEN}Log level set to: info${NC}"; sleep 1 ;;
        3) LOG_LEVEL="warn"; echo -e "${GREEN}Log level set to: warn${NC}"; sleep 1 ;;
        4) LOG_LEVEL="error"; echo -e "${GREEN}Log level set to: error${NC}"; sleep 1 ;;
    esac
}

menu_autoupdate() {
    show_header
    echo -e "${WHITE}Auto-Update Settings${NC}"
    echo ""
    echo -e "Current: ${CYAN}$AUTO_UPDATE${NC}"
    echo ""
    echo "When enabled, X1-Stratus will automatically check for and"
    echo "apply updates daily at 3 AM. A backup is created before"
    echo "each update with automatic rollback on failure."
    echo ""
    echo "Options:"
    echo "  1) Enable Auto-Update"
    echo "  2) Disable Auto-Update"
    echo "  3) Check for Updates Now"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            AUTO_UPDATE="true"
            echo -e "${GREEN}Auto-update enabled${NC}"
            sleep 1
            ;;
        2)
            AUTO_UPDATE="false"
            echo -e "${YELLOW}Auto-update disabled${NC}"
            sleep 1
            ;;
        3)
            echo ""
            x1-stratus update
            echo ""
            read -p "Press Enter to continue..."
            ;;
    esac
}

menu_rpc() {
    show_header
    echo -e "${WHITE}RPC Server Settings${NC}"
    echo ""
    echo "The RPC server allows external applications to query"
    echo "X1-Stratus for blockchain data."
    echo ""
    echo "Current values:"
    echo -e "  RPC Server:     ${CYAN}$ENABLE_RPC${NC}"
    echo -e "  Listen Address: ${CYAN}$RPC_ADDR${NC}"
    echo ""
    echo "Options:"
    echo "  1) Enable RPC Server"
    echo "  2) Disable RPC Server"
    echo "  3) Set Listen Address"
    echo ""
    echo "  0) Back"
    echo ""
    echo -n "Select: "

    read choice
    case $choice in
        1)
            ENABLE_RPC="true"
            echo -e "${GREEN}RPC server enabled${NC}"
            sleep 1
            ;;
        2)
            ENABLE_RPC="false"
            echo -e "${YELLOW}RPC server disabled${NC}"
            sleep 1
            ;;
        3)
            echo ""
            echo "Enter listen address:"
            echo "  :8899         - All interfaces, port 8899"
            echo "  127.0.0.1:8899 - Localhost only"
            echo ""
            echo -n "> "
            read val
            if [[ -n "$val" ]]; then
                RPC_ADDR="$val"
                echo -e "${GREEN}Address set to: $val${NC}"
            fi
            sleep 1
            ;;
    esac
}

view_config() {
    show_header
    echo -e "${WHITE}Current Configuration:${NC}"
    echo ""
    echo "---"
    cat "$CONFIG_FILE" 2>/dev/null || echo "No configuration file found"
    echo "---"
    echo ""
    read -p "Press Enter to continue..."
}

apply_changes() {
    show_header
    echo -e "${YELLOW}Applying changes...${NC}"
    echo ""

    # Save config
    save_config
    echo -e "${GREEN}[✓]${NC} Configuration saved"

    # Update service
    update_service
    echo -e "${GREEN}[✓]${NC} Service file updated"

    # Setup or remove auto-update cron
    if [[ "$AUTO_UPDATE" == "true" ]]; then
        CRON_CMD="0 3 * * * $INSTALL_DIR/scripts/auto-update.sh >> /var/log/x1-stratus-update.log 2>&1"
        (crontab -l 2>/dev/null | grep -v "x1-stratus" ; echo "$CRON_CMD") | crontab -
        echo -e "${GREEN}[✓]${NC} Auto-update cron installed"
    else
        crontab -l 2>/dev/null | grep -v "x1-stratus" | crontab - 2>/dev/null || true
        echo -e "${YELLOW}[!]${NC} Auto-update cron removed"
    fi

    # Restart if running
    if systemctl is-active --quiet $SERVICE_NAME; then
        echo -e "${BLUE}[*]${NC} Restarting service..."
        systemctl restart $SERVICE_NAME
        sleep 3
        if systemctl is-active --quiet $SERVICE_NAME; then
            echo -e "${GREEN}[✓]${NC} Service restarted successfully"
        else
            echo -e "${RED}[✗]${NC} Service failed to start - check logs"
        fi
    else
        echo -e "${YELLOW}[!]${NC} Service not running. Start with: x1-stratus start"
    fi

    echo ""
    read -p "Press Enter to continue..."
}

reset_defaults() {
    show_header
    echo -e "${YELLOW}Reset to Defaults${NC}"
    echo ""
    echo "This will reset all settings to their default values:"
    echo "  - Commitment: confirmed"
    echo "  - Slot threshold: 50"
    echo "  - Log level: info"
    echo "  - Auto-update: true"
    echo "  - RPC server: disabled"
    echo ""
    read -p "Continue? [y/N] " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        DATA_DIR="/mnt/x1-stratus"
        COMMITMENT="confirmed"
        SLOT_THRESHOLD="50"
        LOG_LEVEL="info"
        AUTO_UPDATE="true"
        REDISCOVER_INTERVAL="5"
        ENABLE_RPC="false"
        RPC_ADDR=":8899"
        REFERENCE_ENDPOINTS=""
        NODE_NAME=""
        NODE_WEBSITE=""
        NODE_ICON=""

        save_config
        echo -e "${GREEN}[✓]${NC} Reset to defaults"
        sleep 1
    fi
}

# Main loop
main() {
    load_config

    while true; do
        show_main_menu
        read choice

        case $choice in
            1) menu_identity ;;
            2) menu_branding ;;
            3) menu_network ;;
            4) menu_performance ;;
            5) menu_logging ;;
            6) menu_autostart ;;
            7) menu_autoupdate ;;
            8) menu_rpc ;;
            9) menu_rebuild ;;
            v|V) view_config ;;
            a|A) apply_changes ;;
            r|R) reset_defaults ;;
            0)
                clear
                echo "Configuration saved. Goodbye!"
                exit 0
                ;;
            *)
                echo -e "${RED}Invalid option${NC}"
                sleep 1
                ;;
        esac
    done
}

main
CONFIG_EOF

    chmod +x "$BIN_DIR/x1-stratus-config"
    log_success "x1-stratus-config tool installed"
}

# Setup auto-update
setup_autoupdate() {
    log_info "Setting up auto-update..."

    mkdir -p "$INSTALL_DIR/scripts"

    cat > "$INSTALL_DIR/scripts/auto-update.sh" << 'UPDATE_EOF'
#!/bin/bash
#
# X1-Stratus Auto-Update Script
# Runs daily via cron
#

INSTALL_DIR="/opt/x1-stratus"
CONFIG_DIR="/root/.config/x1-stratus"
LOG_FILE="/var/log/x1-stratus-update.log"

# Load config
[[ -f "$CONFIG_DIR/config.env" ]] && source "$CONFIG_DIR/config.env"

# Check if auto-update is enabled
if [[ "$AUTO_UPDATE" != "true" ]]; then
    exit 0
fi

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

log "Starting auto-update check..."

cd "$INSTALL_DIR/src" || {
    log "ERROR: Source directory not found"
    exit 1
}

# Fetch latest
git fetch origin 2>/dev/null

LOCAL=$(git rev-parse HEAD 2>/dev/null)
REMOTE=$(git rev-parse origin/main 2>/dev/null)

if [[ "$LOCAL" == "$REMOTE" ]]; then
    log "Already up to date"
    exit 0
fi

log "Update available: $LOCAL -> $REMOTE"

# Backup current binary
BACKUP_DIR="$INSTALL_DIR/backups/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"
cp "$INSTALL_DIR/bin/stratus" "$BACKUP_DIR/"
log "Backup created: $BACKUP_DIR"

# Stop service
systemctl stop x1-stratus 2>/dev/null || true
log "Service stopped"

# Update source
git reset --hard origin/main
log "Source updated"

# Build
export PATH=$PATH:/usr/local/go/bin
go build -o "$INSTALL_DIR/bin/stratus" ./cmd/stratus 2>/dev/null

if [[ $? -eq 0 ]]; then
    # Update version
    VERSION=$("$INSTALL_DIR/bin/stratus" --version 2>/dev/null | head -1 | awk '{print $2}' || echo "unknown")
    echo "$VERSION" > "$INSTALL_DIR/.version"

    # Restart service
    systemctl start x1-stratus

    log "SUCCESS: Updated to $VERSION"
else
    log "ERROR: Build failed, rolling back..."
    cp "$BACKUP_DIR/stratus" "$INSTALL_DIR/bin/"
    systemctl start x1-stratus
    log "Rolled back to previous version"
    exit 1
fi
UPDATE_EOF

    chmod +x "$INSTALL_DIR/scripts/auto-update.sh"

    # Add to crontab (daily at 3 AM)
    CRON_CMD="0 3 * * * $INSTALL_DIR/scripts/auto-update.sh >> /var/log/x1-stratus-update.log 2>&1"
    (crontab -l 2>/dev/null | grep -v "x1-stratus" ; echo "$CRON_CMD") | crontab -

    log_success "Auto-update configured (daily at 3 AM)"
}

# Enable and start service
start_service() {
    log_info "Starting X1-Stratus..."

    systemctl enable x1-stratus > /dev/null 2>&1
    systemctl start x1-stratus

    sleep 5

    if systemctl is-active --quiet x1-stratus; then
        log_success "X1-Stratus is running"
    else
        log_warn "Service may still be starting. Check: x1-stratus status"
    fi
}

# Print completion message
print_complete() {
    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                                                               ║${NC}"
    echo -e "${GREEN}║          Installation Complete!                              ║${NC}"
    echo -e "${GREEN}║                                                               ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "X1-Stratus is now running as a lightweight verification node."
    echo "It automatically discovers validators on the X1 network and"
    echo "verifies blocks using a health-checked RPC endpoint pool."
    echo ""
    echo -e "${WHITE}Quick Commands:${NC}"
    echo "  x1-stratus status    - Check service status"
    echo "  x1-stratus logs      - View recent logs"
    echo "  x1-stratus follow    - Follow logs in real-time"
    echo "  x1-stratus config    - Open configuration menu"
    echo "  x1-stratus discover  - Run validator discovery"
    echo "  x1-stratus health    - Check endpoint health"
    echo "  x1-stratus update    - Check for updates"
    echo "  x1-stratus help      - Show all commands"
    echo ""
    echo -e "${WHITE}File Locations:${NC}"
    echo "  Binary:      $INSTALL_DIR/bin/stratus"
    echo "  Config:      $CONFIG_DIR/config.env"
    echo "  Data:        $DATA_DIR"
    echo "  Logs:        journalctl -u x1-stratus"
    echo ""
    echo -e "${CYAN}Thank you for running X1-Stratus!${NC}"
    echo ""
}

# Main installation flow
main() {
    print_banner

    echo -e "${BOLD}X1-Stratus Installer${NC}"
    echo ""
    echo "This will install the X1-Stratus lightweight verification node."
    echo "Unlike full validators, X1-Stratus uses minimal resources (~35MB RAM)"
    echo "by verifying blocks through RPC endpoints discovered on the network."
    echo ""
    echo "Features:"
    echo "  - Automatic validator discovery via gossip/RPC"
    echo "  - Health-checked RPC endpoint pool"
    echo "  - Periodic re-discovery of new validators"
    echo "  - Auto-update with rollback support"
    echo ""

    read -p "Continue with installation? [Y/n] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]?$ ]]; then
        echo "Installation cancelled."
        exit 0
    fi

    local total_steps=9

    print_step 1 $total_steps "Checking prerequisites"
    check_root
    detect_os
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

    print_step 7 $total_steps "Installing service"
    install_service
    install_wrapper
    install_config_tool

    print_step 8 $total_steps "Setting up auto-update"
    setup_autoupdate

    print_step 9 $total_steps "Starting service"
    start_service

    print_complete
}

# Run main
main "$@"
