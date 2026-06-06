# TorrSyncPlayer API Documentation

## API Versioning Policy

- Current version: v1
- API version is URL-based: `/api/v1/...`
- Breaking changes require a new major version (v2, v3, etc.)
- Non-breaking additions are added to the current version
- Deprecated endpoints return a `Deprecation` header
- Minimum support: current version + 1 previous version

## Overview

TorrSyncPlayer provides an HTTP REST API for managing torrents, P2P rooms, and playback synchronization.

- **Base URL:** `http://localhost:8889`
- **API Version:** v1
- **Format:** JSON
- **Authentication:** JWT token (for protected endpoints)
- **Swagger UI:** `http://localhost:8889/swagger/`

## Authentication Flow

1. Register: `POST /api/v1/auth/register` `{username, password}`
2. Login: `POST /api/v1/auth/login` `{username, password}` → returns JWT
3. Use JWT: Include `Authorization: Bearer <token>` in requests
4. Logout: `POST /api/v1/auth/logout` (revokes token)
5. CSRF: For non-JWT requests, obtain token from `GET` response `X-CSRF-Token` header

## Authentication

### POST /api/v1/auth/register

Register a new user.

**Request:**
```json
{
  "username": "user123",
  "password": "securepassword"
}
```

**Response (201):**
```json
{
  "token": "jwt_token_here",
  "user": {
    "id": "uuid",
    "username": "user123",
    "createdAt": 1704067200000
  }
}
```

### POST /api/v1/auth/login

Log in.

**Request:**
```json
{
  "username": "user123",
  "password": "securepassword"
}
```

**Response (200):**
```json
{
  "token": "jwt_token_here",
  "user": {
    "id": "uuid",
    "username": "user123",
    "createdAt": 1704067200000
  }
}
```

### POST /api/v1/auth/logout

Log out (revokes JWT token).

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "message": "Logged out"
}
```

## CSRF Protection

### GET /api/v1/csrf-token

Get a CSRF token for cross-site request forgery protection.

**Response (200):**
```json
{
  "csrfToken": "csrf_token_here"
}
```

Response header: `X-CSRF-Token: csrf_token_here`

## Health Check

### GET /health

Basic health check (no authentication required).

**Response (200):**
```json
{
  "status": "ok",
  "uptime": 3600.5,
  "version": "1.0.0",
  "services": {
    "torrent": "ok",
    "p2p": "ok",
    "sync": "ok"
  }
}
```

### GET /api/v1/health/detailed

Extended health check with service status (requires JWT).

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "status": "ok",
  "services": {
    "torrent": "ok",
    "p2p": "ok",
    "sync": "ok"
  },
  "version": "1.0.0"
}
```

**Response (503) on issues:**
```json
{
  "status": "degraded",
  "services": {
    "torrent": "ok",
    "p2p": "unavailable",
    "sync": "ok"
  },
  "version": "1.0.0"
}
```

## Version

### GET /api/v1/version

Get server version (no authentication required).

**Response (200):**
```json
{
  "version": "1.0.0",
  "commit": "abc123",
  "buildTime": "2025-01-01T00:00:00Z"
}
```

## Metrics

### GET /metrics

Prometheus metrics (no authentication required).

**Response (200):**
```
# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/health"} 42
...
```

## Torrent API

### GET /api/v1/torrents

Get list of all torrents.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Query parameters:**
- `limit` (int, optional) — number of items (default 20, max 100)
- `offset` (int, optional) — offset (default 0)

**Response (200):**
```json
{
  "torrents": [
    {
      "id": "info_hash",
      "name": "Movie Name",
      "size": 1073741824,
      "progress": 0.75,
      "status": "downloading"
    }
  ],
  "totalCount": 10,
  "limit": 20,
  "offset": 0,
  "hasMore": true
}
```

### POST /api/v1/torrents

Add a torrent by magnet link.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "magnetUri": "magnet:?xt=urn:btih:..."
}
```

**Response (201):**
```json
{
  "id": "info_hash",
  "name": "Movie Name",
  "size": 1073741824,
  "progress": 0.0,
  "status": "loading"
}
```

**Errors:**
- `400` — Invalid magnet URI format
- `500` — Internal server error

### DELETE /api/v1/torrents/{id}

Remove a torrent.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "message": "Torrent removed"
}
```

**Errors:**
- `400` — Invalid torrent ID
- `404` — Torrent not found

### GET /api/v1/torrents/{id}/files

Get list of files in a torrent.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Query parameters:**
- `limit` (int, optional) — number of items (default 20, max 100)
- `offset` (int, optional) — offset (default 0)

**Response (200):**
```json
{
  "files": [
    {
      "index": 0,
      "name": "movie.mp4",
      "size": 1073741824
    }
  ],
  "totalCount": 5,
  "limit": 20,
  "offset": 0,
  "hasMore": true
}
```

**Errors:**
- `400` — Invalid torrent ID
- `404` — Torrent not found

### POST /api/v1/torrents/{id}/select

Select a file for streaming.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "fileIndex": 0
}
```

**Response (200):**
```json
{
  "message": "File selected"
}
```

**Errors:**
- `400` — Invalid file index or torrent ID
- `404` — Torrent not found

### GET /api/v1/torrents/{id}/stream

Stream the selected file.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response headers:**
- `Content-Type`: file MIME type
- `Accept-Ranges: bytes` — Range request support

**Supported formats:**
- Video: mp4, mkv, avi, webm, mov, wmv, flv
- Audio: mp3, aac, wav, ogg, flac
- Subtitles: srt, ass, ssa

**Errors:**
- `400` — File not selected or invalid ID
- `404` — Torrent not found

### POST /api/v1/torrents/{id}/buffer/position

Set buffer position for priority downloading.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "position": 120.5
}
```

**Response (200):**
```json
{
  "message": "Buffer position updated"
}
```

### GET /api/v1/torrents/{id}/buffer/info

Get buffer status information.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "position": 120.5,
  "buffered": 0.15,
  "bufferSize": 536870912
}
```

## Room API

### POST /api/v1/rooms

Create a new room.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "name": "My Room",
  "password": "optional_password"
}
```

**Response (201):**
```json
{
  "id": "room_id",
  "name": "My Room",
  "hostId": "peer_id",
  "peerCount": 1
}
```

**Errors:**
- `400` — Invalid room name

### POST /api/v1/rooms/join

Join a room.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "roomId": "room_id",
  "password": "room_password"
}
```

**Response (200):**
```json
{
  "message": "Joined room"
}
```

**Errors:**
- `400` — Invalid room ID
- `401` — Wrong password
- `404` — Room not found

### POST /api/v1/rooms/leave

Leave a room.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "message": "Left room"
}
```

**Errors:**
- `400` — Not in a room

### POST /api/v1/rooms/signal

Send a WebRTC signal.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "roomId": "room_id",
  "signal": [1, 2, 3, ...]
}
```

**Response (200):**
```json
{
  "message": "Signal sent"
}
```

**Errors:**
- `400` — Not in room or invalid ID

### GET /api/v1/rooms/{roomID}/events

Connect to the room's SSE event stream.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response headers:**
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`

**Events:**
- `connected` — Connection established
- `peer_joined` — Peer joined
- `peer_left` — Peer left
- `signal` — WebRTC signal
- `ping` — Keep-alive ping
- `timeout` — Connection timeout

**Example event:**
```
event: peer_joined
data: {"type": "peer_joined", "peerId": "peer_id", "roomId": "room_id"}
```

## Sync API

### POST /api/v1/sync/play

Start synchronized playback.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "isPlaying": true,
  "position": 120.5,
  "duration": 3600.0,
  "timestamp": 1704067200000
}
```

### POST /api/v1/sync/pause

Pause playback.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "isPlaying": false,
  "position": 125.0,
  "duration": 3600.0,
  "timestamp": 1704067205000
}
```

### POST /api/v1/sync/seek

Synchronize seeking.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Request:**
```json
{
  "position": 300.0
}
```

**Response (200):**
```json
{
  "isPlaying": true,
  "position": 300.0,
  "duration": 3600.0,
  "timestamp": 1704067500000
}
```

**Errors:**
- `400` — Invalid position

### GET /api/v1/sync/status

Get current sync status.

**Headers:**
```
Authorization: Bearer <jwt_token>
```

**Response (200):**
```json
{
  "isPlaying": true,
  "position": 120.5,
  "duration": 3600.0,
  "timestamp": 1704067200000
}
```

## Error Codes

| Code | Description |
|------|-------------|
| 400 | Bad Request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not Found |
| 408 | Request Timeout |
| 409 | Conflict |
| 429 | Too Many Requests |
| 500 | Internal Server Error |
| 503 | Service Unavailable |

## Error Format

All errors are returned in JSON format:

```json
{
  "code": 404,
  "message": "Torrent not found"
}
```

## Rate Limiting

- **Auth endpoints:** 10 requests/minute (burst 5)
- **API endpoints:** 60 requests/minute (burst 10)

Response headers:
- `X-RateLimit-Limit` — Request limit
- `X-RateLimit-Remaining` — Remaining requests
- `X-RateLimit-Reset` — Limit reset time

## CORS

API supports CORS for the following origins:
- `http://localhost:*` (development)
- Configurable via `CORS_ORIGINS` environment variable

Allowed methods: `GET, POST, PUT, DELETE, OPTIONS`
Allowed headers: `Content-Type, Authorization, X-Requested-With, X-CSRF-Token, X-Session-ID`

## SSE (Server-Sent Events)

For real-time events, use SSE:

```javascript
const eventSource = new EventSource('/api/v1/rooms/{roomID}/events');

eventSource.addEventListener('peer_joined', (e) => {
  const data = JSON.parse(e.data);
  console.log('Peer joined:', data.peerId);
});

eventSource.addEventListener('signal', (e) => {
  const data = JSON.parse(e.data);
  handleSignal(data);
});
```

## Security

- JWT authentication (HS256, 24h TTL, JTI for revocation)
- bcrypt password hashing (cost=12)
- CSRF protection (token store with TTL 1h)
- Rate limiting
- Security headers (X-Content-Type-Options, X-Frame-Options, HSTS)
- CORS policies
- Input data validation
- TLS 1.2+ support
