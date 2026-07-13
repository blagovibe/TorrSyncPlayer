#!/bin/bash
# run_chaos.sh - Run chaos engineering tests for TorrSyncPlayer
# 
# This script runs various chaos scenarios using Toxiproxy
# to test the resilience of the P2P synchronization system.

set -euo pipefail

# Configuration
TOXIPROXY_VERSION="v2.6.0"
BACKEND_PORT=8889
TOXIPROXY_PORT=8474
TOXIPROXY_API_PORT=8474
RESULTS_DIR="results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Create results directory
mkdir -p "${RESULTS_DIR}/${TIMESTAMP}"

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is required but not installed"
        exit 1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        log_error "jq is required but not installed"
        exit 1
    fi
    
    log_success "All dependencies available"
}

# Build backend
build_backend() {
    log_info "Building backend..."
    cd ../../backend
    make build
    cd ../tests/chaos
    log_success "Backend built"
}

# Start backend server
start_backend() {
    log_info "Starting backend server on port ${BACKEND_PORT}..."
    
    JWT_SECRET="test-jwt-secret-key-for-chaos-testing-min-32-chars" \
    ../../backend/build/torrsyncplayer \
        --port "${BACKEND_PORT}" \
        --auto-tls \
        --data-dir "./data" \
        > "${RESULTS_DIR}/${TIMESTAMP}/backend.log" 2>&1 &
    
    BACKEND_PID=$!
    
    # Wait for backend to be ready
    for i in {1..30}; do
        if curl -k -s "https://localhost:${BACKEND_PORT}/health" > /dev/null 2>&1; then
            log_success "Backend is ready"
            return 0
        fi
        sleep 1
    done
    
    log_error "Backend failed to start"
    kill $BACKEND_PID 2>/dev/null || true
    exit 1
}

# Stop backend
stop_backend() {
    log_info "Stopping backend..."
    kill $BACKEND_PID 2>/dev/null || true
    wait $BACKEND_PID 2>/dev/null || true
}

# Start Toxiproxy
start_toxiproxy() {
    log_info "Starting Toxiproxy..."
    
    docker run -d \
        --name toxiproxy \
        --network host \
        -p ${TOXIPROXY_PORT}:${TOXIPROXY_PORT} \
        -p ${TOXIPROXY_API_PORT}:${TOXIPROXY_API_PORT} \
        ghcr.io/shopify/toxiproxy:${TOXIPROXY_VERSION} \
        > /dev/null
    
    # Wait for Toxiproxy API
    for i in {1..10}; do
        if curl -s "http://localhost:${TOXIPROXY_API_PORT}/proxies" > /dev/null 2>&1; then
            log_success "Toxiproxy is ready"
            return 0
        fi
        sleep 1
    done
    
    log_error "Toxiproxy failed to start"
    exit 1
}

# Stop Toxiproxy
stop_toxiproxy() {
    log_info "Stopping Toxiproxy..."
    docker stop toxiproxy > /dev/null 2>&1 || true
    docker rm toxiproxy > /dev/null 2>&1 || true
}

# Create proxy for backend
create_proxy() {
    log_info "Creating Toxiproxy proxy for backend..."
    
    cat > /tmp/proxy.json <<EOF
{
    "name": "backend",
    "listen": "0.0.0.0:${BACKEND_PORT}",
    "upstream": "localhost:${BACKEND_PORT}",
    "enabled": true
}
EOF
    
    curl -s -X POST "http://localhost:${TOXIPROXY_API_PORT}/proxies" \
        -H "Content-Type: application/json" \
        -d @/tmp/proxy.json > /dev/null
    
    log_success "Proxy created"
}

# Apply toxic
apply_toxic() {
    local toxic_name=$1
    local toxic_type=$2
    local stream=$3
    local attributes=$4
    
    log_info "Applying toxic: ${toxic_name} (${toxic_type})"
    
    cat > /tmp/toxic.json <<EOF
{
    "name": "${toxic_name}",
    "type": "${toxic_type}",
    "stream": "${stream}",
    "attributes": ${attributes}
}
EOF
    
    curl -s -X POST "http://localhost:${TOXIPROXY_API_PORT}/proxies/backend/toxics" \
        -H "Content-Type: application/json" \
        -d @/tmp/toxic.json > /dev/null
}

# Remove toxic
remove_toxic() {
    local toxic_name=$1
    
    curl -s -X DELETE "http://localhost:${TOXIPROXY_API_PORT}/proxies/backend/toxics/${toxic_name}" > /dev/null
}

# Run a chaos scenario
run_scenario() {
    local scenario_file=$1
    local scenario_name=$(basename "${scenario_file}" .json)
    
    log_info "=== Running scenario: ${scenario_name} ==="
    
    # Parse scenario
    local duration=$(jq -r '.duration_seconds' "${scenario_file}")
    local toxics=$(jq -c '.toxics[]' "${scenario_file}")
    
    # Apply all toxics
    echo "${toxics}" | while read -r toxic; do
        local name=$(echo "${toxic}" | jq -r '.name')
        local type=$(echo "${toxic}" | jq -r '.type')
        local stream=$(echo "${toxic}" | jq -r '.stream')
        local attributes=$(echo "${toxic}" | jq -c '.attributes')
        
        apply_toxic "${name}" "${type}" "${stream}" "${attributes}"
    done
    
    # Run verification tests during chaos
    log_info "Running verification tests for ${duration}s..."
    run_verification_tests "${scenario_name}" "${duration}" &
    VERIFICATION_PID=$!
    
    # Wait for scenario duration
    sleep "${duration}"
    
    # Clean up toxics
    echo "${toxics}" | while read -r toxic; do
        local name=$(echo "${toxic}" | jq -r '.name')
        remove_toxic "${name}"
    done
    
    # Wait for verification to complete
    wait $VERIFICATION_PID
    
    log_success "Scenario ${scenario_name} completed"
}

# Run verification tests
run_verification_tests() {
    local scenario_name=$1
    local duration=$2
    local end_time=$(($(date +%s) + duration))
    
    while [ $(date +%s) -lt ${end_time} ]; do
        # Health check
        if ! curl -k -s "https://localhost:${BACKEND_PORT}/health" > /dev/null; then
            log_warning "Health check failed during ${scenario_name}"
        fi
        
        # API checks
        curl -k -s -H "Authorization: Bearer test-token" \
            "https://localhost:${BACKEND_PORT}/api/v1/torrents" > /dev/null || true
        
        curl -k -s -H "Authorization: Bearer test-token" \
            "https://localhost:${BACKEND_PORT}/api/v1/rooms" > /dev/null || true
        
        sleep 5
    done
}

# Main execution
main() {
    local scenarios=(
        "scenarios/network_partition.json"
        "scenarios/latency_injection.json"
        "scenarios/packet_loss.json"
        "scenarios/peer_crash.json"
    )
    
    log_info "Starting chaos engineering tests for TorrSyncPlayer"
    log_info "Results will be saved to ${RESULTS_DIR}/${TIMESTAMP}/"
    
    check_dependencies
    build_backend
    
    # Trap to cleanup on exit
    trap 'stop_backend; stop_toxiproxy; log_info "Cleanup complete"' EXIT
    
    start_backend
    start_toxiproxy
    create_proxy
    
    # Run each scenario
    for scenario in "${scenarios[@]}"; do
        if [ -f "${scenario}" ]; then
            run_scenario "${scenario}"
            
            # Recovery period between scenarios
            log_info "Recovery period (30s)..."
            sleep 30
        else
            log_warning "Scenario file not found: ${scenario}"
        fi
    done
    
    log_success "All chaos scenarios completed!"
    log_info "Results saved to ${RESULTS_DIR}/${TIMESTAMP}/"
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi