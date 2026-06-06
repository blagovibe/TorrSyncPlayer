# Prometheus Metrics Reference

TorrSyncPlayer exposes Prometheus metrics at the `/metrics` endpoint. All metrics use the `torrserver_` prefix.

## Metrics

### Uptime

| Metric | Type | Description |
|--------|------|-------------|
| `torrserver_uptime_seconds` | gauge | Server uptime in seconds |

### Requests

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `torrserver_requests_total` | counter | `status` (success, error) | Total number of HTTP requests |

### Torrents

| Metric | Type | Description |
|--------|------|-------------|
| `torrserver_torrents_active` | gauge | Number of currently active torrents |
| `torrserver_torrents_added_total` | counter | Total number of torrents added (cumulative) |

### Rooms

| Metric | Type | Description |
|--------|------|-------------|
| `torrserver_rooms_active` | gauge | Number of currently active P2P rooms |
| `torrserver_rooms_created_total` | counter | Total number of rooms created (cumulative) |

### Peers

| Metric | Type | Description |
|--------|------|-------------|
| `torrserver_peers_total` | gauge | Number of currently connected peers |

### Synchronization

| Metric | Type | Description |
|--------|------|-------------|
| `torrserver_sync_operations_total` | counter | Total number of sync operations (cumulative) |

### Memory

| Metric | Type | Description |
|--------|------|-------------|
| `torrserver_memory_alloc_bytes` | gauge | Currently allocated memory in bytes |
| `torrserver_memory_sys_bytes` | gauge | Total system memory obtained from the OS in bytes |
| `torrserver_gc_total` | counter | Total number of GC cycles completed (cumulative) |

## Example Output

```
# HELP torrserver_uptime_seconds Uptime in seconds
# TYPE torrserver_uptime_seconds gauge
torrserver_uptime_seconds 3600.50

# HELP torrserver_requests_total Total requests
# TYPE torrserver_requests_total counter
torrserver_requests_total{status="success"} 42
torrserver_requests_total{status="error"} 3

# HELP torrserver_torrents_active Active torrents
# TYPE torrserver_torrents_active gauge
torrserver_torrents_active 5

# HELP torrserver_torrents_added_total Total added torrents
# TYPE torrserver_torrents_added_total counter
torrserver_torrents_added_total 12

# HELP torrserver_rooms_active Active rooms
# TYPE torrserver_rooms_active gauge
torrserver_rooms_active 2

# HELP torrserver_rooms_created_total Total created rooms
# TYPE torrserver_rooms_created_total counter
torrserver_rooms_created_total 4

# HELP torrserver_peers_total Total peers
# TYPE torrserver_peers_total gauge
torrserver_peers_total 6

# HELP torrserver_sync_operations_total Total sync operations
# TYPE torrserver_sync_operations_total counter
torrserver_sync_operations_total 150

# HELP torrserver_memory_alloc_bytes Allocated memory
# TYPE torrserver_memory_alloc_bytes gauge
torrserver_memory_alloc_bytes 12345678

# HELP torrserver_memory_sys_bytes System memory
# TYPE torrserver_memory_sys_bytes gauge
torrserver_memory_sys_bytes 98765432

# HELP torrserver_gc_total Total GC cycles
# TYPE torrserver_gc_total counter
torrserver_gc_total 42
```

## Implementation Details

- Metrics are cached for 5 seconds to reduce overhead on high-traffic servers.
- The metrics singleton is thread-safe using `sync.RWMutex`.
- Memory stats are obtained from Go's `runtime.MemStats`.
