#!/bin/bash

echo "========================================"
echo "TorrSyncPlayer Build Script"
echo "========================================"

echo ""
echo "[1/4] Installing frontend dependencies..."
cd frontend
npm install
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to install frontend dependencies"
    exit 1
fi
cd ..

echo ""
echo "[2/4] Downloading Go dependencies..."
go mod download
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to download Go dependencies"
    exit 1
fi

echo ""
echo "[3/4] Generating Wails bindings..."
wails generate module
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to generate Wails bindings"
    exit 1
fi

echo ""
echo "[4/4] Building application..."
wails build
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build application"
    exit 1
fi

echo ""
echo "========================================"
echo "Build completed successfully!"
echo "Output: build/bin/TorrSyncPlayer"
echo "========================================"
