@echo off
echo ========================================
echo TorrSyncPlayer Build Script
echo ========================================

echo.
echo [1/4] Installing frontend dependencies...
cd frontend
call npm install
if errorlevel 1 (
    echo ERROR: Failed to install frontend dependencies
    exit /b 1
)
cd ..

echo.
echo [2/4] Downloading Go dependencies...
go mod download
if errorlevel 1 (
    echo ERROR: Failed to download Go dependencies
    exit /b 1
)

echo.
echo [3/4] Generating Wails bindings...
wails generate module
if errorlevel 1 (
    echo ERROR: Failed to generate Wails bindings
    exit /b 1
)

echo.
echo [4/4] Building application...
wails build
if errorlevel 1 (
    echo ERROR: Failed to build application
    exit /b 1
)

echo.
echo ========================================
echo Build completed successfully!
echo Output: build/bin/TorrSyncPlayer.exe
echo ========================================
