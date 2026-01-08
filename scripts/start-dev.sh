#!/bin/bash
# start-dev.sh - Start Gibson daemon, agents, and tools for development
#
# Usage:
#   ./scripts/start-dev.sh              # Start daemon, agents, and tools
#   ./scripts/start-dev.sh stop         # Stop everything
#   ./scripts/start-dev.sh status       # Show status
#   ./scripts/start-dev.sh build-tools  # Build and install OSS tools
#
# Requirements:
#   - Gibson binary built: make bin
#   - Agents built with latest SDK
#   - OSS tools repo cloned: git clone https://github.com/zero-day-ai/gibson-oss-tools

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GIBSON_DIR="$(dirname "$SCRIPT_DIR")"
GIBSON_BIN="$GIBSON_DIR/bin/gibson"
AGENTS_DIR="/home/anthony/Code/zero-day.ai/enterprise/agents"
TOOLS_DIR="/home/anthony/Code/zero-day.ai/opensource/gibson-oss-tools"
LOG_DIR="/tmp/gibson-dev"

# =============================================================================
# DEVELOPMENT/DEBUG ENVIRONMENT - ENABLE ALL VERBOSE LOGGING
# =============================================================================
export GIBSON_LOG_LEVEL=debug
export GIBSON_DEBUG=true
export LOG_LEVEL=debug

# Go slog level
export SLOG_LEVEL=debug

# gRPC verbose logging
export GRPC_GO_LOG_VERBOSITY_LEVEL=99
export GRPC_GO_LOG_SEVERITY_LEVEL=info

# etcd client logging
export ETCD_CLIENT_DEBUG=true

# Registry endpoint for agents
export GIBSON_REGISTRY_ENDPOINTS=localhost:2379

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_debug() { echo -e "${BLUE}[DEBUG]${NC} $1"; }

# Stop everything
stop_all() {
    log_info "Stopping all Gibson processes..."

    # Stop agents
    pkill -f "k8skiller" 2>/dev/null && log_info "Stopped k8skiller" || true
    pkill -f "bishop" 2>/dev/null && log_info "Stopped bishop" || true
    pkill -f "carl" 2>/dev/null && log_info "Stopped carl" || true
    pkill -f "crease" 2>/dev/null && log_info "Stopped crease" || true
    pkill -f "whistler" 2>/dev/null && log_info "Stopped whistler" || true

    # Stop tools (match tool binaries from gibson-oss-tools)
    for tool in subfinder httpx nuclei naabu katana; do
        pkill -f "$tool" 2>/dev/null && log_info "Stopped $tool" || true
    done

    # Stop daemon
    if [ -f "$GIBSON_BIN" ]; then
        "$GIBSON_BIN" daemon stop 2>/dev/null && log_info "Stopped daemon" || true
    fi

    # Clean up stale files
    rm -f ~/.gibson/daemon.pid ~/.gibson/daemon.json 2>/dev/null || true

    log_info "All processes stopped"
}

# Build OSS tools
build_tools() {
    log_info "Building OSS tools from $TOOLS_DIR..."

    if [ ! -d "$TOOLS_DIR" ]; then
        log_warn "OSS tools directory not found: $TOOLS_DIR"
        log_info "Clone it with: git clone https://github.com/zero-day-ai/gibson-oss-tools $TOOLS_DIR"
        return 1
    fi

    cd "$TOOLS_DIR"

    if [ -f "build.sh" ]; then
        log_debug "Running build.sh..."
        ./build.sh 2>&1 | while read line; do
            echo "  $line"
        done
        log_info "OSS tools built successfully"
    else
        log_error "build.sh not found in $TOOLS_DIR"
        return 1
    fi
}

# Install OSS tools to Gibson
install_tools() {
    log_info "Installing OSS tools to Gibson..."

    if [ ! -d "$TOOLS_DIR/bin" ]; then
        log_warn "OSS tools bin directory not found. Run build first."
        return 1
    fi

    # Install each tool binary
    local tools_bin="$TOOLS_DIR/bin"
    local gibson_tools="$HOME/.gibson/tools/bin"

    mkdir -p "$gibson_tools"

    for tool in "$tools_bin"/*; do
        if [ -f "$tool" ] && [ -x "$tool" ]; then
            local name=$(basename "$tool")
            cp "$tool" "$gibson_tools/$name"
            log_debug "Installed $name"
        fi
    done

    log_info "OSS tools installed to $gibson_tools"
}

# Start a tool
start_tool() {
    local name=$1
    local port=$2
    local binary="$HOME/.gibson/tools/bin/$name"

    if [ ! -f "$binary" ]; then
        log_warn "Tool binary not found: $binary (skipping)"
        return 1
    fi

    # Check if already running
    if pgrep -f "$binary" > /dev/null 2>&1; then
        log_warn "$name already running"
        return 0
    fi

    log_info "Starting tool $name on port $port..."

    # Start tool with registry endpoint
    nohup "$binary" --port "$port" > "$LOG_DIR/$name.log" 2>&1 &

    sleep 1

    # Verify it's running
    if pgrep -f "$binary" > /dev/null 2>&1; then
        log_info "$name started"
        return 0
    else
        log_error "$name failed to start. Check $LOG_DIR/$name.log"
        return 1
    fi
}

# Start all tools
start_tools() {
    log_info "Starting OSS tools..."

    mkdir -p "$LOG_DIR"

    # Start commonly used tools on different ports
    # These are the tools most agents need for E2E testing
    local port=50100
    for tool in subfinder httpx nuclei naabu katana; do
        if [ -f "$HOME/.gibson/tools/bin/$tool" ]; then
            start_tool "$tool" $port || true
            port=$((port + 1))
        fi
    done
}

# Start daemon
start_daemon() {
    log_info "Starting Gibson daemon with DEBUG logging..."
    log_debug "GIBSON_LOG_LEVEL=$GIBSON_LOG_LEVEL"
    log_debug "GIBSON_DEBUG=$GIBSON_DEBUG"

    # Check if binary exists
    if [ ! -f "$GIBSON_BIN" ]; then
        log_error "Gibson binary not found at $GIBSON_BIN"
        log_error "Run 'make bin' first"
        exit 1
    fi

    # Check if already running
    if "$GIBSON_BIN" daemon status 2>/dev/null | grep -q "Running:  true"; then
        log_warn "Daemon already running"
        return 0
    fi

    # Create log directory
    mkdir -p "$LOG_DIR"

    # Start daemon in background with verbose flag
    log_debug "Starting: $GIBSON_BIN daemon start --verbose"
    nohup "$GIBSON_BIN" daemon start --verbose > "$LOG_DIR/daemon.log" 2>&1 &
    DAEMON_PID=$!

    # Wait for daemon to start
    log_info "Waiting for daemon to start (PID: $DAEMON_PID)..."
    for i in {1..10}; do
        sleep 1
        if "$GIBSON_BIN" daemon status 2>/dev/null | grep -q "Running:  true"; then
            log_info "Daemon started successfully"
            log_debug "Log file: $LOG_DIR/daemon.log"
            return 0
        fi
    done

    log_error "Daemon failed to start. Check $LOG_DIR/daemon.log"
    cat "$LOG_DIR/daemon.log" | tail -30
    exit 1
}

# Start an agent
start_agent() {
    local name=$1
    local port=$2
    local binary="$AGENTS_DIR/$name/$name"

    if [ ! -f "$binary" ]; then
        log_warn "Agent binary not found: $binary (skipping)"
        return 1
    fi

    # Check if already running
    if pgrep -f "$name" > /dev/null 2>&1; then
        log_warn "$name already running"
        return 0
    fi

    log_info "Starting $name on port $port with DEBUG logging..."
    log_debug "GIBSON_REGISTRY_ENDPOINTS=$GIBSON_REGISTRY_ENDPOINTS"

    # Start agent with registry endpoint and debug env
    cd "$AGENTS_DIR/$name"
    nohup "./$name" --port "$port" > "$LOG_DIR/$name.log" 2>&1 &

    sleep 2

    # Verify it's running
    if pgrep -f "$name" > /dev/null 2>&1; then
        log_info "$name started"
        log_debug "Log file: $LOG_DIR/$name.log"
        return 0
    else
        log_error "$name failed to start. Check $LOG_DIR/$name.log"
        cat "$LOG_DIR/$name.log" | tail -15
        return 1
    fi
}

# Start all agents
start_agents() {
    log_info "Starting agents..."

    # Create log directory
    mkdir -p "$LOG_DIR"

    # Start agents on different ports
    start_agent "k8skiller" 50071 || true
    start_agent "bishop" 50072 || true
    start_agent "carl" 50073 || true
    start_agent "crease" 50074 || true
    start_agent "whistler" 50075 || true
}

# Show status
show_status() {
    echo ""
    log_info "=== Gibson Daemon Status ==="
    "$GIBSON_BIN" daemon status 2>/dev/null || echo "Daemon not running"

    echo ""
    log_info "=== Registered Agents ==="
    "$GIBSON_BIN" agent list 2>/dev/null || echo "No agents or daemon not running"

    echo ""
    log_info "=== Registered Tools ==="
    "$GIBSON_BIN" tool list 2>/dev/null || echo "No tools or daemon not running"

    echo ""
    log_info "=== Running Processes ==="
    ps aux | grep -E "gibson daemon|k8skiller|bishop|carl|crease|whistler|subfinder|httpx|nuclei|naabu|katana" | grep -v grep || echo "No processes running"

    echo ""
    log_info "=== Log Files ==="
    ls -la "$LOG_DIR"/*.log 2>/dev/null || echo "No log files"

    echo ""
    log_info "=== Environment (Debug Settings) ==="
    echo "  GIBSON_LOG_LEVEL=$GIBSON_LOG_LEVEL"
    echo "  GIBSON_DEBUG=$GIBSON_DEBUG"
    echo "  SLOG_LEVEL=$SLOG_LEVEL"
    echo "  GIBSON_REGISTRY_ENDPOINTS=$GIBSON_REGISTRY_ENDPOINTS"
    echo "  GRPC_GO_LOG_VERBOSITY_LEVEL=$GRPC_GO_LOG_VERBOSITY_LEVEL"
}

# Tail logs
tail_logs() {
    log_info "Tailing all logs (Ctrl+C to stop)..."
    tail -f "$LOG_DIR"/*.log
}

# Main
case "${1:-start}" in
    start)
        stop_all
        sleep 1
        start_daemon
        sleep 2
        start_agents
        start_tools
        sleep 2
        show_status
        echo ""
        log_info "To tail logs: $0 logs"
        ;;
    stop)
        stop_all
        ;;
    status)
        show_status
        ;;
    restart)
        stop_all
        sleep 1
        start_daemon
        sleep 2
        start_agents
        start_tools
        sleep 2
        show_status
        ;;
    logs)
        tail_logs
        ;;
    build-tools)
        build_tools
        install_tools
        ;;
    install-tools)
        install_tools
        ;;
    *)
        echo "Usage: $0 {start|stop|status|restart|logs|build-tools|install-tools}"
        echo ""
        echo "Commands:"
        echo "  start         - Stop any running processes and start daemon + agents + tools"
        echo "  stop          - Stop all Gibson processes"
        echo "  status        - Show current status of daemon, agents, and tools"
        echo "  restart       - Restart everything"
        echo "  logs          - Tail all log files"
        echo "  build-tools   - Build and install OSS tools from gibson-oss-tools repo"
        echo "  install-tools - Install pre-built OSS tools to ~/.gibson/tools/bin"
        echo ""
        echo "OSS Tools repo: https://github.com/zero-day-ai/gibson-oss-tools"
        echo "Expected at: $TOOLS_DIR"
        echo ""
        echo "Environment (set for verbose/debug):"
        echo "  GIBSON_LOG_LEVEL=debug"
        echo "  GIBSON_DEBUG=true"
        echo "  SLOG_LEVEL=debug"
        echo "  GRPC_GO_LOG_VERBOSITY_LEVEL=99"
        exit 1
        ;;
esac
