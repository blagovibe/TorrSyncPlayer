# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Stack

- **Backend:** Go 1.25+, anacrolix/torrent, pion/webrtc v4
- **Frontend:** C++17, Qt 6.5+, libmpv
- **Build:** Make (backend), CMake (frontend)

## Project Structure

```
TorrSyncPlayer/
├── backend/           # Go backend (HTTP API server)
│   ├── cmd/server/    # Entry point
│   ├── internal/      # Services (torrent, p2p, sync, api)
│   └── pkg/logger/    # Logging package
└── frontend/          # Qt/C++ frontend
    ├── src/           # Source files
    ├── resources/     # Icons and resources
    └── CMakeLists.txt # Build configuration
```

## Build & Dev Commands

```bash
# Backend
cd backend
make build           # Build Go backend
make run             # Run backend server
make test            # Run Go tests
make clean           # Clean build artifacts

# Frontend
cd frontend
./build.sh           # Build Qt frontend (Linux/macOS)
build.bat            # Build Qt frontend (Windows)

# Root
make all             # Build both backend and frontend
make clean           # Clean both
```

## Architecture

### Backend (Go)
- HTTP REST API server on port 8889
- `TorrentService` — torrent management + HTTP streaming
- `P2PService` — WebRTC peer connections, room management
- `SyncService` — playback sync with latency compensation
- Services are independent (no DI container)

### Frontend (Qt/C++)
- `MpvWidget` — libmpv video player widget
- `NetworkManager` — HTTP client for backend API
- `TorrentModel` — data model for torrent list
- `MainWindow` — main application window
- `RoomDialog` — create/join room dialog
- `SystemTray` — system tray integration

### Communication
- HTTP REST API (JSON)
- Server-Sent Events (SSE) for real-time room events

## Code Conventions

- **Go:** Standard `gofmt`/`go vet`. Use `logger.Info/Warn/Error/Debug` from `pkg/logger`.
- **Error wrapping:** Always use `fmt.Errorf("...: %w", err)`.
- **C++:** Qt naming conventions, camelCase for methods.
- **Comments:** Code comments in Russian.

## Key Files

- `backend/cmd/server/main.go` — Backend entry point
- `backend/internal/api/router.go` — API route definitions
- `frontend/src/main.cpp` — Frontend entry point
- `frontend/src/mainwindow.cpp` — Main window implementation
