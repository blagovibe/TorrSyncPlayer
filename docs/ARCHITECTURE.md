# TorrSyncPlayer Architecture

## Contents

1. [Architecture Overview](#architecture-overview)
2. [Component Diagram](#component-diagram)
3. [Communication Flow](#communication-flow)
4. [Data Model](#data-model)
5. [Security](#security)

---

## Architecture Overview

TorrSyncPlayer is a desktop application for streaming media content via torrents with the ability to synchronize playback between users.

The architecture follows a client-server model with P2P elements:

- **Backend (Go)** — HTTP API server managing torrents, P2P rooms, and synchronization
- **Frontend (Qt/C++)** — desktop application with a video player based on libmpv
- **P2P (WebRTC)** — direct peer-to-peer connection for synchronization data exchange

### Sub-documents

- [Backend Architecture](ARCHITECTURE_BACKEND.md) — detailed backend package structure, service interactions, HTTP API layer, buffering
- [Frontend Architecture](ARCHITECTURE_FRONTEND.md) — detailed frontend module structure, main window architecture, video player, backend communication
- [P2P/WebRTC Architecture](ARCHITECTURE_P2P.md) — detailed P2P service components, WebRTC connection flow, event system, authentication

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
│  │  │  │ - anacrolix/ │  │ - pion/webrtc│  │ - Play/Pause/Seek  │  │   │    │
│  │  │  │   torrent    │  │ - Rooms      │  │ - Latency comp.    │  │   │    │
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
        │                    P2P (WebRTC DataChannel)               │
        │                                                          │
        │    Peer A ◄──────────────────────────────────► Peer B    │
        │                                                          │
        │    - Playback position synchronization                   │
        │    - play/pause/seek state exchange                      │
        │    - STUN for NAT traversal                              │
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
   │  WebRTC DataChannel ◄──────────────────────────►│
   │  (P2P sync data)       │                        │
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
