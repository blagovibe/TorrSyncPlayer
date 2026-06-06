# Backend Architecture

## Package Structure

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
│   ├── p2p/              # P2P service
│   │   └── service.go    # WebRTC connections
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
│   ├── version/          # Version
│   │   └── version.go    # Version info
│   │
│   └── interfaces.go     # Service interfaces
│
└── pkg/logger/           # Logger
    └── logger.go         # Structured logging
```

## Service Interaction

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

## HTTP API Layer

**Routing** (based on [go-chi/chi](https://github.com/go-chi/chi)):

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
| `/api/v1/torrents/{id}/stream` | GET | Stream file | JWT |
| `/api/v1/torrents/{id}/buffer/position` | POST | Set buffer position | JWT |
| `/api/v1/torrents/{id}/buffer/info` | GET | Buffer info | JWT |
| `/api/v1/rooms` | POST | Create room | JWT |
| `/api/v1/rooms/join` | POST | Join room | JWT |
| `/api/v1/rooms/leave` | POST | Leave room | JWT |
| `/api/v1/rooms/signal` | POST | WebRTC signal | JWT |
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

## Synchronization Layer

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

## Buffering Layer

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

## Internal Backend Structures

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
    RequireAuth bool
}
```

**Peer** (internal):
```go
type Peer struct {
    ID            string
    UserID        string
    Username      string
    Connection    *webrtc.PeerConnection
    DataChannel   *webrtc.DataChannel
    LastHeartbeat time.Time
    Authenticated bool
}
```
