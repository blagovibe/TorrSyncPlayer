# TorrSyncPlayer Architecture

## Contents

1. [Architecture Overview](#architecture-overview)
2. [Component Diagram](#component-diagram)
3. [Communication Flow](#communication-flow)
4. [Data Model](#data-model)
5. [Security](#security)
6. [Backend Architecture](#backend-architecture)
7. [Frontend Architecture](#frontend-architecture)
8. [Rooms & Real-Time Synchronization](#rooms--real-time-synchronization-server-brokered)

---

## Architecture Overview

TorrSyncPlayer is a desktop application for streaming media content via torrents with the ability to synchronize playback between users.

The architecture follows a client-server model with P2P elements:

- **Backend (Go)** — HTTP API server managing torrents, P2P rooms, and synchronization
- **Frontend (Qt/C++)** — desktop application with a video player based on libmpv
- **Rooms & Sync (server-brokered SSE)** — playback synchronization relayed by the backend over Server-Sent Events

---

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TorrSyncPlayer                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        Frontend (Qt/C++)                            │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────────┐ │    │
│  │  │  MainWindow   │  │  MpvWidget   │  │     NetworkManager        │ │    │
│  │  │              │  │  (libmpv)    │  │  - HTTP REST API          │ │    │
│  │  │ - UI layout  │  │              │  │  - SSE events             │ │    │
│  │  │ - User input │  │ - Playback   │  │  - Retry logic            │ │    │
│  │  │ - State mgmt │  │ - Rendering  │  │  - Exponential backoff    │ │    │
│  │  └──────┬───────┘  └──────┬───────┘  └─────────────┬─────────────┘ │    │
│  │         │                 │                         │               │    │
│  │  ┌──────┴───────┐  ┌──────┴───────┐  ┌─────────────┴─────────────┐ │    │
│  │  │TorrentManager│  │  RoomManager │  │       TorrentModel         │ │    │
│  │  │              │  │              │  │  - Data model for list     │ │    │
│  │  │ - Add/Remove │  │ - Create/Join│  │  - Qt model/view           │ │    │
│  │  │ - Select file│  │ - Leave room │  │                            │ │    │
│  │  └──────────────┘  └──────────────┘  └───────────────────────────┘ │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                         │
│                          HTTP REST API / SSE                                 │
│                                    │                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                          Backend (Go)                               │    │
│  │                                                                     │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │                      API Layer (chi router)                   │   │    │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ │   │    │
│  │  │  │   Auth     │ │  Torrent   │ │   Room     │ │   Sync     │ │   │    │
│  │  │  │  Handlers  │ │  Handlers  │ │  Handlers  │ │  Handlers  │ │   │    │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘ │   │    │
│  │  │                                                              │   │    │
│  │  │  Middleware: SecurityHeaders → Recovery → CORS → Logger → CSRF│   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  │                                    │                                 │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │                      Service Layer                            │   │    │
│  │  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐  │   │    │
│  │  │  │   Torrent    │  │     P2P      │  │       Sync         │  │   │    │
│  │  │  │   Service    │  │   Service    │  │      Service       │  │   │    │
│  │  │  │              │  │              │  │                    │  │   │    │
│  │  │  │ - anacrolix/ │  │ - Rooms      │  │ - Play/Pause/Seek  │  │   │    │
│  │  │  │   torrent    │  │ - SSE events │  │ - Latency comp.    │  │   │    │
│  │  │  │ - Magnet     │  │ - Peers      │  │ - Smooth adjust    │  │   │    │
│  │  │  │ - Streaming  │  │ - JWT auth   │  │                    │  │   │    │
│  │  │  └──────────────┘  └──────────────┘  └────────────────────┘  │   │    │
│  │  │  ┌──────────────┐  ┌──────────────┐                          │   │    │
│  │  │  │   Buffer     │  │   Storage    │                          │   │    │
│  │  │  │   Service    │  │   Service    │                          │   │    │
│  │  │  │              │  │              │                          │   │    │
│  │  │  │ - LRU cache  │  │ - In-memory  │                          │   │    │
│  │  │  │ - Priorities │  │ - Users      │                          │   │    │
│  │  │  └──────────────┘  └──────────────┘                          │   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  │                                    │                                 │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │                    Cross-cutting Concerns                     │   │    │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ │   │    │
│  │  │  │   Auth     │ │ Validation │ │  Metrics   │ │  Logging   │ │   │    │
│  │  │  │  Package   │ │  Package   │ │ (Prometheus│ │  Package   │ │   │    │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘ │   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

        ┌──────────────────────────────────────────────────────────┐
        │              Server-Brokered Synchronization             │
        │                                                          │
        │    Peer A ──┐                            ┌── Peer B       │
        │             │   SSE event stream        │                │
        │             └──► Backend (rooms) ◄──────┘                │
        │                                                          │
        │    - Playback position synchronization                   │
        │    - play/pause/seek state relayed via SSE               │
        │    - no direct peer-to-peer data path                    │
        └──────────────────────────────────────────────────────────┘
```

---

## Communication Flow

### Adding a Torrent and Playback

```
Frontend                 Backend                 Torrent Client
   │                        │                        │
   │  POST /torrents        │                        │
   │  {magnetUri}           │                        │
   │───────────────────────►│                        │
   │                        │  AddMagnet()           │
   │                        │───────────────────────►│
   │                        │                        │
   │                        │  GotInfo()             │
   │                        │◄───────────────────────│
   │                        │                        │
   │  201 {torrentInfo}     │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  GET /torrents/{id}/files                       │
   │───────────────────────►│                        │
   │  200 {files[]}         │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  POST /torrents/{id}/select                     │
   │  {fileIndex}           │                        │
   │───────────────────────►│                        │
   │                        │  SetPriority()         │
   │                        │───────────────────────►│
   │  200 OK                │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  GET /torrents/{id}/stream                      │
   │───────────────────────►│                        │
   │                        │  ServeFile()           │
   │                        │───────────────────────►│
   │  200 (video stream)    │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  MpvWidget.play(url)   │                        │
   │  (local call)          │                        │
```

### Room Creation and Synchronization

```
User A (Host)            Backend                  User B (Peer)
   │                        │                        │
   │  POST /rooms           │                        │
   │  {name, password}      │                        │
   │───────────────────────►│                        │
   │  201 {roomInfo}        │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  GET /rooms/{id}/events│                        │
   │  (SSE connect)         │                        │
   │◄═══════════════════════│                        │
   │                        │                        │
   │                        │    POST /rooms/join    │
   │                        │    {roomId, password}  │
   │                        │◄───────────────────────│
   │                        │                        │
   │  SSE: peer_joined      │    200 OK              │
   │◄═══════════════════════│───────────────────────►│
   │                        │                        │
   │                        │    GET /rooms/{id}/events
   │                        │    (SSE connect)       │
   │                        │═══════════════════════►│
   │                        │                        │
   │  POST /sync/play       │                        │
   │───────────────────────►│                        │
   │  200 {syncStatus}      │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  BroadcastSync (SSE) ──────────────────────────►│
   │  (sync events relayed to all room peers)        │
```

---

## Data Model

### Core Structures

**TorrentInfo:**
```go
type TorrentInfo struct {
    ID       string  `json:"id"`       // Info hash (hex)
    Name     string  `json:"name"`     // Torrent name
    Progress float64 `json:"progress"` // Download progress (0-1)
    Status   string  `json:"status"`   // loading/downloading/seeding
    Size     int64   `json:"size"`     // Size in bytes
}
```

**FileInfo:**
```go
type FileInfo struct {
    Index int    `json:"index"` // File index
    Name  string `json:"name"`  // File name
    Size  int64  `json:"size"`  // Size in bytes
}
```

**RoomInfo:**
```go
type RoomInfo struct {
    ID        string `json:"id"`        // Room ID
    Name      string `json:"name"`      // Room name
    HostID    string `json:"hostId"`    // Host peer ID
    PeerCount int    `json:"peerCount"` // Number of peers
}
```

**SyncStatus:**
```go
type SyncStatus struct {
    IsPlaying bool    `json:"isPlaying"` // Playback active
    Position  float64 `json:"position"`  // Position in seconds
    Duration  float64 `json:"duration"`  // Duration in seconds
    Timestamp int64   `json:"timestamp"` // Unix timestamp (ms)
}
```

**P2PEvent:**
```go
type P2PEvent struct {
    Type string      `json:"type"` // Event type
    Data interface{} `json:"data"` // Event data
}
```

**User:**
```go
type User struct {
    ID           string `json:"id"`
    Username     string `json:"username"`
    PasswordHash string `json:"-"`       // bcrypt hash
    CreatedAt    int64  `json:"createdAt"`
}
```

### Entity Relationships

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Entity Relationships                              │
│                                                                             │
│  ┌─────────────┐       1:N      ┌─────────────┐                           │
│  │   Torrent   │───────────────►│    File     │                           │
│  └─────────────┘                └─────────────┘                           │
│                                                                             │
│  ┌─────────────┐       1:N      ┌─────────────┐                           │
│  │    Room     │───────────────►│    Peer     │                           │
│  └─────────────┘                └─────────────┘                           │
│                                                                             │
│  ┌─────────────┐       1:1      ┌─────────────┐                           │
│  │    User     │───────────────►│  SyncStatus │                           │
│  └─────────────┘                └─────────────┘                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Security

### Authentication
- JWT tokens for user authentication (HS256, 24h TTL)
- bcrypt hashing for room passwords (cost=12)
- JTI (JWT ID) for token revocation
- Tokens have an expiration time

### API Protection
- CSRF tokens for cross-site request forgery protection (TTL 1h)
- Rate limiting (10 req/min for auth, 60 req/min for API)
- CORS policies
- Security headers (X-Content-Type-Options, X-Frame-Options, HSTS)
- All input data validation
- TLS 1.2+ support

### Thread Safety
- `sync.RWMutex` in each service
- Buffered channels for events
- Graceful shutdown with timeouts
- Context for operation cancellation

---

## Backend Architecture

### Package Structure

```
backend/
├── cmd/server/           # Entry point (main.go)
│
├── internal/
│   ├── api/              # HTTP API layer
│   │   ├── router.go     # Routing (chi)
│   │   ├── handlers.go   # Request handlers
│   │   ├── middleware.go # Middleware (CORS, CSRF, Rate Limit)
│   │   ├── response.go   # Response formatting
│   │   └── paths.go      # API path constants
│   │
│   ├── auth/             # Authentication
│   │   ├── auth.go       # JWT logic
│   │   ├── handlers.go   # Register/Login/Logout
│   │   ├── middleware.go # JWT middleware
│   │   ├── store.go      # User storage
│   │   └── revocation.go # Token revocation
│   │
│   ├── torrent/          # Torrent service
│   │   └── service.go    # Torrent management
│   │
│   ├── p2p/              # Rooms & real-time event service
│   │   └── service.go    # Room management, SSE event broker
│   │
│   ├── sync/             # Synchronization service
│   │   └── service.go    # Playback synchronization
│   │
│   ├── buffer/           # Buffering
│   │   └── service.go    # LRU cache, priorities
│   │
│   ├── storage/          # Storage
│   │   └── storage.go    # In-memory storage
│   │
│   ├── models/           # Data models
│   │   └── types.go      # Common types
│   │
│   ├── validation/       # Validation
│   │   └── validation.go # Validation functions
│   │
│   ├── errors/           # Error handling
│   │   └── errors.go     # Error types
│   │
│   ├── metrics/          # Prometheus metrics
│   │   └── metrics.go    # Metric definitions
│   │
│   ├── constants/        # Constants
│   │   └── constants.go  # Magic numbers
│   │
│   ├── persistence/      # File persistence (users, revoked tokens)
│   │   └── persistence.go # JSON-file storage
│   │
│   ├── utils/            # Utility functions
│   │   └── id.go         # ID generation
│   │
│   ├── integration/      # Integration tests
│   │
│   ├── contract/         # Pact provider contract tests
│   │
│   ├── version/          # Version
│   │   └── version.go    # Version info
│   │
│   └── interfaces.go     # Service interfaces
│
└── pkg/
    ├── logger/           # Logger
    │   └── logger.go     # Structured logging
    └── response/         # Response helpers
        └── response.go   # JSON/error formatting
```

### Service Interaction

Services are designed as independent components without a DI container. Communication happens through interfaces defined in [`internal/interfaces.go`](../backend/internal/interfaces.go):

```
┌─────────────────────────────────────────────────────────────────┐
│                        main.go                                  │
│                                                                 │
│  1. Initialize services:                                        │
│     - logger.Init()                                             │
│     - authService = auth.NewAuthService(jwtSecret)              │
│     - torrentService = torrent.NewService(bufferService)              │
│     - p2pService = p2p.NewService(authService)                  │
│     - syncService = sync.NewService()                           │
│     - bufferService = buffer.NewService()                       │
│                                                                 │
│  2. Create router:                                              │
│     - router := api.NewRouter(RouterConfig{...})                │
│                                                                 │
│  3. Start HTTP server:                                          │
│     - http.ListenAndServe(port, router)                         │
│                                                                 │
│  4. Graceful shutdown:                                          │
│     - torrentService.Close()                                    │
│     - p2pService.Close()                                        │
│     - syncService.Close()                                       │
│     - bufferService.Close()                                     │
└─────────────────────────────────────────────────────────────────┘
```

**Principles:**
- Each service has its own mutex for thread safety
- Services do not depend on each other directly
- Inter-service communication happens through HTTP API from the frontend
- Graceful shutdown with timeouts for correct termination

### HTTP API Layer

**Routing** (based on go-chi/chi):

| Path | Method | Description | Authentication |
|------|--------|-------------|----------------|
| `/health` | GET | Basic health check | No |
| `/api/v1/version` | GET | Server version | No |
| `/metrics` | GET | Prometheus metrics | No |
| `/api/v1/csrf-token` | GET | Get CSRF token | No |
| `/swagger/` | GET | Swagger UI | No |
| `/api/v1/auth/register` | POST | Register | No |
| `/api/v1/auth/login` | POST | Login | No |
| `/api/v1/auth/logout` | POST | Logout | No |
| `/api/v1/torrents` | GET | List torrents | JWT |
| `/api/v1/torrents` | POST | Add torrent | JWT |
| `/api/v1/torrents/{id}` | DELETE | Remove torrent | JWT |
| `/api/v1/torrents/{id}/files` | GET | List files | JWT |
| `/api/v1/torrents/{id}/select` | POST | Select file | JWT |
| `/api/v1/torrents/{id}/stream` | GET | Stream file | Signed stream ticket (public; libmpv cannot send JWT headers) |
| `/api/v1/torrents/{id}/buffer/position` | POST | Set buffer position | JWT |
| `/api/v1/torrents/{id}/buffer/info` | GET | Buffer info | JWT |
| `/api/v1/rooms` | POST | Create room | JWT |
| `/api/v1/rooms/join` | POST | Join room | JWT |
| `/api/v1/rooms/leave` | POST | Leave room | JWT |
| `/api/v1/rooms/signal` | POST | Relay sync signal to room peers | JWT |
| `/api/v1/rooms/{roomID}/events` | GET | SSE events | JWT |
| `/api/v1/sync/play` | POST | Sync play | JWT |
| `/api/v1/sync/pause` | POST | Sync pause | JWT |
| `/api/v1/sync/seek` | POST | Sync seek | JWT |
| `/api/v1/sync/status` | GET | Sync status | JWT |
| `/api/v1/health/detailed` | GET | Detailed health check | JWT |

**Middleware pipeline** (order matters):

```
Request → SecurityHeaders → Recovery → CORS → Logger → CSRF → RateLimit → Auth → Handler
```

### Synchronization Layer

**Components** ([`internal/sync/service.go`](../backend/internal/sync/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                      Sync Service                           │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                   SyncStatus                         │    │
│  │  - IsPlaying: bool                                  │    │
│  │  - Position: float64 (seconds)                      │    │
│  │  - Duration: float64 (seconds)                      │    │
│  │  - Timestamp: int64 (Unix ms)                       │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  Methods:                                                   │
│  - Play() → SyncStatus                                      │
│  - Pause() → SyncStatus                                     │
│  - Seek(position) → SyncStatus                              │
│  - GetStatus() → SyncStatus                                 │
│  - SyncWithLatency(peerStatus, latencyMs) → SyncStatus      │
│  - UpdatePosition(position) → error                         │
└─────────────────────────────────────────────────────────────┘
```

**Latency compensation algorithm:**

```
1. Get remote peer status (position, timestamp, isPlaying)
2. Calculate expected position:
   - If playing: expected = position + elapsed - latency
   - If paused: expected = position
3. Smooth adjustment:
   - If |diff| > maxPositionJump (2 sec): position += diff * 0.3
   - Else: position = expected
4. Synchronize play/pause state
```

### Buffering Layer

**Components** ([`internal/buffer/service.go`](../backend/internal/buffer/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                      Buffer Service                         │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                    LRU Cache                         │    │
│  │  - DefaultMaxBufferSize: 512 MB                     │    │
│  │  - DefaultBufferPercent: 10%                        │    │
│  │  - DefaultBufferDuration: 60 sec                    │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │               Piece Priorities                       │    │
│  │  - PiecePriorityNow: 4 (immediate download)         │    │
│  │  - PiecePriorityHigh: 3 (high priority)             │    │
│  │  - PiecePriorityNormal: 2 (normal)                  │    │
│  │  - PiecePriorityReadahead: 1 (readahead)            │    │
│  │  - PiecePriorityNone: 0 (do not download)           │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Internal Backend Structures

**Room** (internal):
```go
type Room struct {
    ID          string
    Name        string
    HostID      string
    HostUserID  string
    Password    string           // bcrypt hash
    Peers       map[string]*Peer
    CreatedAt   time.Time
}
```

**Peer** (internal):
```go
type Peer struct {
    ID            string
    UserID        string
    Username      string
    LastHeartbeat time.Time
}
```

---

## Frontend Architecture

### Module Structure

```
frontend/src/
├── main.cpp              # Entry point
├── mainwindow.h/.cpp     # Main window
├── mpvwidget.h/.cpp      # Video player (libmpv)
├── networkmanager.h/.cpp # HTTP client
├── torrentmodel.h/.cpp   # Torrent data model
├── torrentmanager.h/.cpp # Torrent manager
├── roommanager.h/.cpp    # Room manager
├── roomdialog.h/.cpp     # Create/join dialog
├── systemtray.h/.cpp     # System tray
├── inetworkmanager.h     # Network manager interface
├── utils.h/.cpp          # Utilities
├── test_torrentmodel.cpp # TorrentModel tests
└── test_networkmanager.cpp # NetworkManager tests
```

### Main Window Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MainWindow                                      │
│                                                                             │
│  ┌────────────────────────────┐  ┌────────────────────────────────────────┐ │
│  │       Left Panel           │  │           Right Panel                  │ │
│  │                            │  │                                        │ │
│  │  ┌──────────────────────┐  │  │  ┌──────────────────────────────────┐ │ │
│  │  │   TorrentModel       │  │  │  │         MpvWidget                │ │ │
│  │  │   (QListView)        │  │  │  │                                  │ │ │
│  │  └──────────────────────┘  │  │  │  - mpv_handle                    │ │ │
│  │  ┌──────────────────────┐  │  │  │  - mpv_render_context            │ │ │
│  │  │   File List          │  │  │  │  - OpenGL rendering              │ │ │
│  │  │   (QListView)        │  │  │  └──────────────────────────────────┘ │ │
│  │  └──────────────────────┘  │  │  ┌──────────────────────────────────┐ │ │
│  │  ┌──────────────────────┐  │  │  │    Control Panel                 │ │ │
│  │  │  [Magnet Input]      │  │  │  │  [Play/Pause] [Seek] [Time]      │ │ │
│  │  │  [Add Button]        │  │  │  └──────────────────────────────────┘ │ │
│  │  └──────────────────────┘  │  │  ┌──────────────────────────────────┐ │ │
│  └────────────────────────────┘  │  │    Room Panel                    │ │ │
│                                  │  │  [Create] [Join] [Leave]         │ │ │
│                                  └────────────────────────────────────────┘ │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                         Status Bar                                     ││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
```

### Backend Communication

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         NetworkManager                                      │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        HTTP REST API                                 │    │
│  │                                                                     │    │
│  │  Torrent API:    POST/GET/DELETE /api/v1/torrents/*                 │    │
│  │  Room API:       POST /api/v1/rooms/*                               │    │
│  │  Sync API:       POST/GET /api/v1/sync/*                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     SSE (Server-Sent Events)                        │    │
│  │                                                                     │    │
│  │  GET /api/v1/rooms/{roomID}/events                                  │    │
│  │                                                                     │    │
│  │  Events: connected, peer_joined, peer_left, signal, ping, timeout   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        Retry Logic                                   │    │
│  │                                                                     │    │
│  │  - Exponential backoff: delay = baseDelay * 2^attempt               │    │
│  │  - Max retries: 3 (configurable)                                    │    │
│  │  - Base delay: 1000ms (configurable)                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Request pattern:**

```
1. Frontend calls a NetworkManager method (e.g., addTorrent)
2. NetworkManager builds an HTTP request and sends it
3. On response, parses JSON
4. Emits a signal (e.g., torrentAdded)
5. MainWindow is connected to the signal and updates the UI
```

### Video Player (libmpv)

**Components** ([`frontend/src/mpvwidget.h`](../frontend/src/mpvwidget.h)):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MpvWidget                                       │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        libmpv Core                                   │    │
│  │                                                                     │    │
│  │  mpv_handle ─── mpv_create()                                        │    │
│  │  mpv_render_context ─── mpv_render_context_create()                 │    │
│  │                                                                     │    │
│  │  Commands: play, pause, seek, getProperty                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Event Processing                                  │    │
│  │                                                                     │    │
│  │  mpv_event → processMpvEvent() → eventBuffer → emit signals        │    │
│  │                                                                     │    │
│  │  Events: positionChanged, durationChanged, playbackFinished, error  │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Thread Safety                                     │    │
│  │                                                                     │    │
│  │  QMutex for mpv_handle protection                                   │    │
│  │  QTimer for event processing in the main thread                     │    │
│  │  Seek debounce to prevent leaks during fast seeking                 │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Thread safety:**
- All mpv calls are protected by `QMutex`
- mpv events are buffered and emitted in the main thread
- Seek debounce prevents leaks during fast seeking

---

## Rooms & Real-Time Synchronization (Server-Brokered)

Synchronization between viewers is **server-brokered**: the backend relays all
room events over Server-Sent Events (SSE) to every participant's per-user
session channel. There is no direct peer-to-peer data path — clients connect to
the backend over REST (commands) and SSE (events). This keeps the topology
simple, NAT-friendly and debuggable.

### Service Components

**Components** ([`internal/p2p/service.go`](../backend/internal/p2p/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                     Room/Sync Service                        │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │    Room     │  │    Peer     │  │      Session        │  │
│  │             │  │             │  │                     │  │
│  │ - ID        │  │ - ID        │  │ - userID            │  │
│  │ - Name      │  │ - UserID    │  │ - currentRoom       │  │
│  │ - HostID    │  │ - Username  │  │ - eventChan (SSE)   │  │
│  │ - Password  │  │ - LastHB    │  │                     │  │
│  │ - Peers     │  │             │  │                     │  │
│  │  HostUserID │  │             │  │                     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Event System (SSE)                      │    │
│  │                                                     │    │
│  │  eventChan (buffered 100) → SSE stream → Frontend   │    │
│  │                                                     │    │
│  │  Events: room_created, peer_joined, peer_left,      │    │
│  │          signal, sync, ping                         │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### Peer Authentication

- JWT token is passed when joining a room
- Token is validated via `authService.ValidateTokenWithRevocation()`
- Signals and sync commands are relayed through the server to all peers in the room

### SSE Event Types

| Event Type | Description |
|------------|-------------|
| `room_created` | Room was created |
| `peer_joined` | A peer joined the room |
| `peer_left` | A peer left the room |
| `signal` | Opaque sync signal relayed between clients |
| `sync` | Playback command (play/pause/seek) broadcast to all peers |
| `ping` | Keep-alive ping |
