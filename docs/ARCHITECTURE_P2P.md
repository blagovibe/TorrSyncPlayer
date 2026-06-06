# P2P/WebRTC Architecture

## P2P Service Components

**Components** ([`internal/p2p/service.go`](../backend/internal/p2p/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                       P2P Service                           │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │    Room     │  │    Peer     │  │    WebRTC API       │  │
│  │             │  │             │  │                     │  │
│  │ - ID        │  │ - ID        │  │ - PeerConnection    │  │
│  │ - Name      │  │ - UserID    │  │ - DataChannel       │  │
│  │ - HostID    │  │ - Username  │  │ - ICE candidates    │  │
│  │ - Password  │  │ - Conn      │  │ - STUN config       │  │
│  │ - Peers     │  │ - DataCh    │  │                     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Event System (SSE)                      │    │
│  │                                                     │    │
│  │  eventChan (buffered 100) → SSE stream → Frontend   │    │
│  │                                                     │    │
│  │  Events: room_created, peer_joined, peer_left,      │    │
│  │          signal, ice_candidate, connected,           │    │
│  │          disconnected, failed, ping                  │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Peer Authentication

- JWT token is passed when joining a room
- Token is validated via `authService.ValidateToken()`
- Peer is marked as `Authenticated` after successful validation

## STUN Servers for NAT Traversal

- `stun:stun.l.google.com:19302`
- `stun:stun1.l.google.com:19302`

## WebRTC Connection Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         WebRTC Connection Flow                               │
│                                                                             │
│  Host                              Peer                                      │
│    │                                  │                                     │
│    │  1. Create PeerConnection        │                                     │
│    │  2. Create DataChannel           │                                     │
│    │  3. Create Offer (SDP)           │                                     │
│    │                                  │                                     │
│    │  ──── SDP Offer (via SSE) ──────►│                                     │
│    │                                  │  4. Create PeerConnection           │
│    │                                  │  5. Set Remote Description          │
│    │                                  │  6. Create Answer (SDP)             │
│    │                                  │                                     │
│    │  ◄── SDP Answer (via SSE) ──────│                                     │
│    │  7. Set Remote Description       │                                     │
│    │                                  │                                     │
│    │  ──── ICE Candidates ───────────►│                                     │
│    │  ◄─── ICE Candidates ────────────│                                     │
│    │                                  │                                     │
│    │  ════ DataChannel Open ═════════│                                     │
│    │                                  │                                     │
│    │  ════ Sync Data (P2P) ═════════►│                                     │
│    │  ◄═══ Sync Data (P2P) ══════════│                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Internal Structures

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

## SSE Event Types

| Event Type | Description |
|------------|-------------|
| `room_created` | Room was created |
| `peer_joined` | A peer joined the room |
| `peer_left` | A peer left the room |
| `signal` | WebRTC signal (SDP offer/answer) |
| `ice_candidate` | ICE candidate exchange |
| `connected` | P2P connection established |
| `disconnected` | P2P connection lost |
| `failed` | P2P connection failed |
| `ping` | Keep-alive ping |
| `timeout` | Connection timeout |
