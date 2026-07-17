# Prometheus Metrics Reference

TorrSyncPlayer exposes Prometheus metrics at the `/metrics` endpoint. All metrics use the `torrsyncplayer_` prefix.

## Metrics

### Uptime

| Metric | Type | Description |
|--------|------|-------------|
| `torrsyncplayer_uptime_seconds` | gauge | Server uptime in seconds |

### Requests

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `torrsyncplayer_requests_total` | counter | `status` (success, error) | Total number of HTTP requests |

### Torrents

| Metric | Type | Description |
|--------|------|-------------|
| `torrsyncplayer_torrents_active` | gauge | Number of currently active torrents |
| `torrsyncplayer_torrents_added_total` | counter | Total number of torrents added (cumulative) |

### Rooms

| Metric | Type | Description |
|--------|------|-------------|
| `torrsyncplayer_rooms_active` | gauge | Number of currently active P2P rooms |
| `torrsyncplayer_rooms_created_total` | counter | Total number of rooms created (cumulative) |

### Peers

| Metric | Type | Description |
|--------|------|-------------|
| `torrsyncplayer_peers_total` | gauge | Number of currently connected peers |

### Synchronization

| Metric | Type | Description |
|--------|------|-------------|
| `torrsyncplayer_sync_operations_total` | counter | Total number of sync operations (cumulative) |

### Memory

| Metric | Type | Description |
|--------|------|-------------|
| `torrsyncplayer_memory_alloc_bytes` | gauge | Currently allocated memory in bytes |
| `torrsyncplayer_memory_sys_bytes` | gauge | Total system memory obtained from the OS in bytes |
| `torrsyncplayer_gc_total` | counter | Total number of GC cycles completed (cumulative) |

## Example Output

```
# HELP torrsyncplayer_uptime_seconds Uptime in seconds
# TYPE torrsyncplayer_uptime_seconds gauge
torrsyncplayer_uptime_seconds 3600.50

# HELP torrsyncplayer_requests_total Total requests
# TYPE torrsyncplayer_requests_total counter
torrsyncplayer_requests_total{status="success"} 42
torrsyncplayer_requests_total{status="error"} 3

# HELP torrsyncplayer_torrents_active Active torrents
# TYPE torrsyncplayer_torrents_active gauge
torrsyncplayer_torrents_active 5

# HELP torrsyncplayer_torrents_added_total Total added torrents
# TYPE torrsyncplayer_torrents_added_total counter
torrsyncplayer_torrents_added_total 12

# HELP torrsyncplayer_rooms_active Active rooms
# TYPE torrsyncplayer_rooms_active gauge
torrsyncplayer_rooms_active 2

# HELP torrsyncplayer_rooms_created_total Total created rooms
# TYPE torrsyncplayer_rooms_created_total counter
torrsyncplayer_rooms_created_total 4

# HELP torrsyncplayer_peers_total Total peers
# TYPE torrsyncplayer_peers_total gauge
torrsyncplayer_peers_total 6

# HELP torrsyncplayer_sync_operations_total Total sync operations
# TYPE torrsyncplayer_sync_operations_total counter
torrsyncplayer_sync_operations_total 150

# HELP torrsyncplayer_memory_alloc_bytes Allocated memory
# TYPE torrsyncplayer_memory_alloc_bytes gauge
torrsyncplayer_memory_alloc_bytes 12345678

# HELP torrsyncplayer_memory_sys_bytes System memory
# TYPE torrsyncplayer_memory_sys_bytes gauge
torrsyncplayer_memory_sys_bytes 98765432

# HELP torrsyncplayer_gc_total Total GC cycles
# TYPE torrsyncplayer_gc_total counter
torrsyncplayer_gc_total 42
```

## Implementation Details

- Metrics are cached for 5 seconds to reduce overhead on high-traffic servers.
- The metrics singleton is thread-safe using `sync.RWMutex`.
- Memory stats are obtained from Go's `runtime.MemStats`.
