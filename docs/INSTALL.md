# TorrSyncPlayer Installation Guide

## Contents

1. [System Requirements](#system-requirements)
2. [Installing Dependencies](#installing-dependencies)
3. [Building from Source](#building-from-source)
4. [Installing from Releases](#installing-from-releases)
5. [Docker](#docker)
6. [Configuration](#configuration)
7. [Updating](#updating)
8. [Uninstallation](#uninstallation)

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
# Install Go 1.25+
wget https://go.dev/dl/go1.25.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

#### macOS

```bash
brew install go@1.24
```

#### Windows

Download and install Go from the [official site](https://go.dev/dl/).

### Frontend (Qt + libmpv)

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

The executable will be in `backend/build/` or `backend/bin/`.

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
make all
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

## Docker

### Build Image

```bash
docker build -t torrsyncplayer:latest .
```

### Run Container

```bash
docker run -d \
    --name torrsyncplayer \
    -p 8889:8889 \
    torrsyncplayer:latest
```

### Docker Compose

```bash
# Run backend only
docker-compose up -d

# Run backend + Prometheus + Grafana
docker-compose --profile monitoring up -d
```

### Docker Compose Services

| Service | Port | Description |
|---------|------|-------------|
| backend | 8889 | TorrSyncPlayer backend |
| prometheus | 9090 | Prometheus metrics (profile: monitoring) |
| grafana | 3000 | Grafana dashboards (profile: monitoring) |

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

### Grafana Password Rotation

1. Stop Grafana container
2. Set new `GRAFANA_PASSWORD` in `.env`
3. Restart Grafana container
4. The new password takes effect on startup

## Updating

### Update from Source

```bash
git pull origin main
make clean
make all
```

### Update Docker

```bash
docker pull torrsyncplayer:latest
docker-compose up -d
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

### Docker

```bash
docker-compose down
docker rmi torrsyncplayer:latest
```
