# TorrSyncPlayer

P2P synchronized video player via torrent protocol.

TorrSyncPlayer allows users to watch videos together in real-time using torrent streaming and WebRTC peer-to-peer connections.

## Features

- 🎬 Stream video from torrents
- 👥 Synchronized playback with friends
- 🔗 P2P connections via WebRTC
- 💬 Built-in chat
- 🎮 Master/Slave synchronization model

## Production Deployment

### TURN Servers

For production deployment, TURN servers are **required** to ensure reliable P2P connections for all users. Without TURN, approximately 10-20% of users behind symmetric NATs or corporate firewalls will be unable to connect.

See [TURN_SETUP.md](docs/TURN_SETUP.md) for detailed instructions on:
- Setting up your own coturn server
- Using cloud TURN services (Twilio, Xirsys, Metered.ca)
- Configuring TURN in TorrSyncPlayer
- Security best practices

Quick configuration via environment variables:
```bash
TURN_SERVER_URLS=turn:your-turn-server.com:3478
TURN_USERNAME=torrsync
TURN_CREDENTIAL=your_secure_password
```

## Tech Stack

- **Backend:** Go
- **Frontend:** React + TypeScript
- **Desktop Framework:** Wails v2
- **Torrent Client:** anacrolix/torrent
- **P2P:** pion/webrtc

## Requirements

- Go 1.21+
- Node.js 20+
- Wails CLI

## Installation

### Install Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Clone and setup

```bash
git clone https://github.com/yourusername/TorrSyncPlayer.git
cd TorrSyncPlayer
make install
```

## Development

```bash
# Run in development mode
wails dev

# Build for production
wails build
```

## Project Structure

```
TorrSyncPlayer/
├── main.go              # Application entry point
├── app.go               # Main app structure
├── services.go          # Torrent service
├── p2p_service.go       # P2P service (WebRTC)
├── sync_service.go      # Playback synchronization
├── config/              # Configuration packages
│   └── turn_config.go   # TURN server configuration
├── docs/                # Documentation
│   └── TURN_SETUP.md    # TURN server setup guide
├── wails.json           # Wails configuration
├── Makefile             # Build commands
├── go.mod               # Go module
├── frontend/            # React frontend
│   ├── src/
│   │   ├── components/  # React components
│   │   ├── hooks/       # Custom hooks
│   │   ├── services/    # API services
│   │   ├── types/       # TypeScript types
│   │   └── utils/       # Utilities
│   └── wailsjs/         # Wails generated files
└── .github/workflows/   # CI/CD
```

## Building

```bash
# Build for current platform
make build

# Build for specific platform
make build-windows
make build-linux
make build-macos

# Build for all platforms
make build-all
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
