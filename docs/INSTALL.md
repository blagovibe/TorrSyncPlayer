# TorrSyncPlayer Installation Guide

## Contents

1. [System Requirements](#system-requirements)
2. [Installing Dependencies](#installing-dependencies)
3. [Building from Source](#building-from-source)
4. [Installing from Releases](#installing-from-releases)
5. [Configuration](#configuration)
6. [Updating](#updating)
7. [Uninstallation](#uninstallation)

## System Requirements

### Minimum Requirements

- **OS:** Windows 10+, Ubuntu 20.04+, macOS 12+
- **RAM:** 8 GB (torrent data is stored in memory)
- **Disk:** 500 MB for installation
- **Network:** stable internet connection

> **Note:** All torrent data is stored in-memory. Make sure you have enough RAM for the content you are loading.

### Recommended Requirements

- **OS:** Windows 11, Ubuntu 22.04+, macOS 13+
- **RAM:** 16 GB (for comfortable work with large torrents)
- **Disk:** SSD with 1 GB free space
- **Network:** 100 Mbps or higher

## Installing Dependencies

### Backend (Go)

#### Ubuntu/Debian

```bash
# Install Go 1.26+
wget https://go.dev/dl/go1.26.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

#### macOS

```bash
# Requires Go 1.26 or newer
brew install go
```

#### Windows

Download and install Go from the [official site](https://go.dev/dl/).

### Frontend (Qt + libmpv)

> **Note:** `libmpv` is a required runtime dependency for video playback. If it
> is not found at build time, the application compiles but **playback is
> disabled** — selecting a file shows a warning instead of playing video.
> Install `libmpv-dev` (or `mpv` on macOS/Windows) before building to enable it.

#### Ubuntu/Debian

```bash
sudo apt update
sudo apt install -y \
    build-essential \
    cmake \
    ninja-build \
    qt6-base-dev \
    qt6-multimedia-dev \
    libmpv-dev \
    libgl1-mesa-dev
```

#### macOS

```bash
brew install qt@6 mpv cmake ninja
```

#### Windows

1. Install Qt 6.5+ from the [official site](https://www.qt.io/download)
2. Install libmpv via vcpkg or download binaries

## Building from Source

### Clone the Repository

```bash
git clone https://github.com/blagovibe/TorrSyncPlayer.git
cd TorrSyncPlayer
```

### Build Backend

```bash
cd backend
make build
```

The executable will be in `backend/build/`.

### Build Frontend

#### Linux/macOS

```bash
cd frontend
mkdir -p build
cd build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja
```

#### Windows

```bash
cd frontend
mkdir build
cd build
cmake .. -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
```

### Build Everything

```bash
# Build backend
cd backend && make build

# Build frontend
cd frontend
mkdir -p build
cd build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja
```

## Installing from Releases

### Download

1. Go to the [Releases](https://github.com/blagovibe/TorrSyncPlayer/releases) page
2. Download the archive for your OS:
   - `TorrSyncPlayer-linux-x64.AppImage` — Linux x64
   - `TorrSyncPlayer-portable.exe` — Windows x64
   - `TorrSyncPlayer-macos-arm64.dmg` — macOS ARM64
3. Extract to a convenient location

### Installation

#### Linux

```bash
chmod +x TorrSyncPlayer-linux-x64.AppImage
./TorrSyncPlayer-linux-x64.AppImage
```

#### macOS

```bash
hdiutil attach TorrSyncPlayer-macos-arm64.dmg
cp -R /Volumes/TorrSyncPlayer/TorrSyncPlayer.app /Applications/
hdiutil detach /Volumes/TorrSyncPlayer
```

#### Windows

Extract the archive and run `TorrSyncPlayer.exe`.

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8889 | HTTP server port |
| `JWT_SECRET` | (empty) | JWT token secret |
| `LOG_LEVEL` | info | Log level (debug/info/warn/error) |
| `LOG_FORMAT` | text | Log format (text/json) |
| `TLS_CERT` | (empty) | Path to TLS certificate |
| `TLS_KEY` | (empty) | Path to TLS key |
| `DATA_DIR` | data | Directory for persistent data (users, revoked tokens, room & sync state, and disk storage when `--disk-storage` is set); empty = in-memory only |
| `CORS_ORIGINS` | (empty → `http://localhost:8889`, `https://localhost:8889`, `http://127.0.0.1:8889`, `https://127.0.0.1:8889`) | Allowed CORS origins (comma-separated) |
| `MEMORY_CAPACITY` | 4GB | Memory storage capacity per-user |
| `DISK_STORAGE` | false | Persist torrent pieces to disk under `DATA_DIR` (requires `DATA_DIR`) |
| `ENV` | development | Environment (development/production) |
| `TRUSTED_PROXIES` | (empty) | Comma-separated trusted proxy IPs |

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 8889 | HTTP server port |
| `--jwt-secret` | (empty) | JWT token secret |
| `--tls` | false | Enable TLS |
| `--auto-tls` | false | Generate self-signed certificate |
| `--enable-profiling` | false | Enable pprof on port 6060 |

### Running with Parameters

```bash
# With custom port
./server --port 8080

# With TLS
./server --tls --tls-cert /path/to/cert.pem --tls-key /path/to/key.pem

# With auto-generated TLS certificate
./server --auto-tls

# With profiling
./server --enable-profiling
```

### Running as a Service

#### Linux (systemd)

Create `/etc/systemd/system/torrsyncplayer.service`:

```ini
[Unit]
Description=TorrSyncPlayer Server
After=network.target

[Service]
Type=simple
User=torrsyncplayer
WorkingDirectory=/opt/TorrSyncPlayer
ExecStart=/opt/TorrSyncPlayer/build/torrsyncplayer
Restart=on-failure
RestartSec=5
Environment=PORT=8889
Environment=JWT_SECRET=your-secret-here

[Install]
WantedBy=multi-user.target
```

Then enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable torrsyncplayer
sudo systemctl start torrsyncplayer
```

#### macOS (launchd)

Create `~/Library/LaunchAgents/com.torrsyncplayer.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.torrsyncplayer</string>
    <key>ProgramArguments</key>
    <array>
        <string>/path/to/torrsyncplayer</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Then load:

```bash
launchctl load ~/Library/LaunchAgents/com.torrsyncplayer.plist
```

#### Windows (NSSM)

```bash
nssm install TorrSyncPlayer "C:\path\to\server.exe"
nssm start TorrSyncPlayer
```

## P2P Configuration

### Storage Backend

By default, torrent pieces are kept **in memory** (bounded by `MEMORY_CAPACITY`,
max 256 GB). To persist pieces to disk instead, enable disk storage:

```bash
# CLI flag
./torrsyncplayer-server --data-dir ./data --disk-storage

# or environment variable
export DATA_DIR=./data
export DISK_STORAGE=true
```

When disk storage is enabled, pieces are written under `<DATA_DIR>/torrents`.
`--disk-storage` requires `--data-dir` to be set.

### Server-Brokered Synchronization

Room synchronization is relayed by the backend over Server-Sent Events (SSE);
there is no direct peer-to-peer data path, so **no STUN/TURN servers or
additional port forwarding are required** for synchronization. The default
backend port (`PORT`, default `8889`) must be reachable by clients.

## Docker Deployment

The backend server can be run as a Docker container:

```bash
# Build the image
cd backend
docker build -t torrsyncplayer-server .

# Run with in-memory storage
docker run -d --name torrsyncplayer \
  -p 8889:8889 \
  -e JWT_SECRET="your-secure-secret-at-least-32-chars" \
  -e LOG_LEVEL=info \
  torrsyncplayer-server

# Run with persistent data directory
docker run -d --name torrsyncplayer \
  -p 8889:8889 \
  -v torrsync-data:/data \
  -e JWT_SECRET="your-secure-secret-at-least-32-chars" \
  -e DATA_DIR=/data \
  -e LOG_LEVEL=info \
  torrsyncplayer-server

# Run with TLS
docker run -d --name torrsyncplayer \
  -p 8889:8889 \
  -v /path/to/certs:/certs:ro \
  -e JWT_SECRET="your-secure-secret-at-least-32-chars" \
  -e TLS_CERT=/certs/cert.pem \
  -e TLS_KEY=/certs/key.pem \
  -e LOG_LEVEL=info \
  torrsyncplayer-server --tls
```

## Updating

### Update from Source

```bash
git pull origin main
cd backend && make clean && make build
```

### Update from Releases

1. Download the new version
2. Stop the current version
3. Replace the files
4. Start the new version

## Uninstallation

### Linux

```bash
sudo systemctl stop torrsyncplayer
sudo systemctl disable torrsyncplayer
sudo rm /etc/systemd/system/torrsyncplayer.service
sudo rm -rf /opt/TorrSyncPlayer
```

### macOS

```bash
launchctl unload ~/Library/LaunchAgents/com.torrsyncplayer.plist
rm ~/Library/LaunchAgents/com.torrsyncplayer.plist
rm -rf /Applications/TorrSyncPlayer
```

### Windows

1. Stop the service: `nssm stop TorrSyncPlayer`
2. Remove the service: `nssm remove TorrSyncPlayer`
3. Delete the installation directory
