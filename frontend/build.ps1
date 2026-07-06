# TorrSyncPlayer Build Script for Windows
# Cross-platform build equivalent to build.sh

param(
    [string]$BuildType = "Release"
)

$ErrorActionPreference = "Stop"

# Colors for output
function Write-Info { Write-Host "[INFO] $args" -ForegroundColor Green }
function Write-Warn { Write-Host "[WARN] $args" -ForegroundColor Yellow }
function Write-ErrorMsg { Write-Host "[ERROR] $args" -ForegroundColor Red }

# Check dependencies
Write-Info "Checking dependencies..."
if (-not (Get-Command cmake -ErrorAction Silentlyently)) {
    Write-ErrorMsg "cmake not found. Install Qt6 and CMake."
    exit 1
}

# Build directory
$BuildDir = "build"
if (Test-Path $BuildDir) {
    Write-Warn "Build directory exists, cleaning..."
    Remove-Item -Recurse -Force $BuildDir
}

New-Item -ItemType Directory -Path $BuildDir | Out-Null
Set-Location $BuildDir

# Configure CMake
Write-Info "Configuring CMake ($BuildType)..."
cmake .. -DCMAKE_BUILD_TYPE=$BuildType -DCMAKE_EXPORT_COMPILE_COMMANDS=ON

if ($LASTEXITCODE -ne 0) {
    Write-ErrorMsg "CMake configuration failed"
    exit 1
}

# Build
Write-Info "Building..."
cmake --build . --config $BuildType

if ($LASTEXITCODE -ne 0) {
    Write-ErrorMsg "Build failed"
    exit 1
}

Write-Info "Build completed successfully!"
Write-Info "Executable: $(Get-Location)\TorrSyncPlayer.exe"
