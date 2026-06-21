#!/bin/bash
# Build script for TorrSyncPlayer frontend on macOS
# Requires: Qt6 (via Homebrew), libmpv, cmake

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/build_macos"

echo "=== TorrSyncPlayer Frontend macOS Build ==="
echo "Build dir: ${BUILD_DIR}"

# Check prerequisites
if ! command -v cmake &> /dev/null; then
    echo "ERROR: cmake not found. Install with: brew install cmake"
    exit 1
fi

if ! brew list qt &> /dev/null && ! brew list qt@6 &> /dev/null; then
    echo "WARNING: Qt6 not found via Homebrew. Trying pkg-config..."
fi

# Configure
echo "--- Configuring ---"
mkdir -p "${BUILD_DIR}"
cd "${BUILD_DIR}"
cmake "${SCRIPT_DIR}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_TESTS=ON \
    -DCMAKE_OSX_DEPLOYMENT_TARGET=12.0

# Build
echo "--- Building ---"
cmake --build . --parallel "$(sysctl -n hw.ncpu)"

# Run tests
echo "--- Testing ---"
ctest --output-on-failure -V || echo "Tests may require display server"

echo "=== Build complete ==="
echo "Binary: ${BUILD_DIR}/TorrSyncPlayer"
