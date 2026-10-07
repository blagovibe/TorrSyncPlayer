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
- **Authentication:** `X-Access-Token` header, printed by the server at startup
- **Swagger UI:** `http://localhost:8889/swagger/`

## Authentication

There is no registration and no login. When the server starts it generates a
random access token and prints it to the log. That token is the only thing
required to call the API, and it is regenerated on every start — nothing is
stored, nothing expires, and there is nothing to revoke.

Send it on every protected request:

```
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
```

`X-Client-ID` is not an account. It identifies one connected player inside a
room for the lifetime of the run; nothing is stored against it.

A request with a missing or wrong token is answered `401`. Comparison is
constant time.

### Getting the token

The server prints it on startup:

```
access token (share this with friends along with the server address)  token=<hex>
```

Restarting the server produces a new token. To use the token from a script or
another machine, read it from the log or pass it to the player with
`--access-token`.

### No CSRF token

There is no CSRF token to fetch. A cross-origin page cannot read the access
token, and cannot set a custom header on the request without a CORS preflight
that the server does not approve — so the token itself is the protection.

### Media playback

`libmpv` fetches media on its own and cannot attach request headers, so
`POST /api/v1/torrents/{id}/stream-ticket` returns a short-lived signed ticket
that `GET /api/v1/torrents/{id}/stream?ticket=...` accepts instead. The ticket
is bound to one torrent and expires in minutes.

## Health Check

### GET /health

Basic health check (no authentication required).

**Response (200):**
```json
{
  "status": "ok"
}
```

### GET /api/v1/health/detailed

Extended health check with service status (requires the access token).

**Headers:**
```
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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

**Authentication:** This endpoint is public but requires a signed stream
ticket (issued via `POST /api/v1/torrents/{id}/stream-ticket`). libmpv cannot
attach headers to its HTTP fetch, so the ticket is passed as a query
parameter:

```
GET /api/v1/torrents/{id}/stream?ticket=<signed_ticket>
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

### POST /api/v1/torrents/{id}/stream-ticket

Request a short-lived, HMAC-signed stream ticket used to authenticate the
`/stream` endpoint without an access-token header (libmpv cannot attach headers to
its own HTTP fetches).

**Headers:**
```
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
```

**Response (200):**
```json
{
  "ticket": "signed_ticket_here"
}
```

The ticket is valid for `StreamTicketTTL` (5 minutes) and is bound to the
requesting user and torrent ID.

### POST /api/v1/torrents/{id}/buffer/position

Set buffer position for priority downloading.

**Headers:**
```
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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

Relay a sync signal to all peers in the room (server-brokered over SSE).

**Headers:**
```
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
```

**Response headers:**
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`

**Events:**
- `room_created` — Room created
- `peer_joined` — Peer joined
- `peer_left` — Peer left
- `signal` — Sync signal relayed to peers
- `sync` — Playback command broadcast
- `ping` — Keep-alive ping

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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
X-Access-Token: <token printed at startup>
X-Client-ID: <any id for this player>
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
Allowed headers: `Content-Type, X-Access-Token, X-Client-ID, X-Requested-With`

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
