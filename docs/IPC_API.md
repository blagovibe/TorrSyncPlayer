# TorrSyncPlayer — IPC Message API

This document describes the IPC (Inter-Process Communication) message protocol between the Electron renderer process and the main process.

## Overview

All IPC messages use `ipcRenderer.invoke()` (request/response) or `ipcRenderer.send()` (fire-and-forget). The preload script (`preload.cjs`) exposes APIs via `contextBridge.exposeInMainWorld()`.

## Security

All IPC handlers validate the sender origin:
- Only `http://127.0.0.1` and `http://localhost` origins are accepted
- `file://` protocol is rejected
- Non-loopback hostnames are rejected
- Port must match the static server or dev server URL

---

## `torrsyncElectronTorrent` API

Exposed as `window.torrsyncElectronTorrent`.

### `addMagnet(magnetLink: string): Promise<TorrentInstance>`

Add a torrent from a magnet link.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `magnetLink` | `string` | Magnet URI (max 8000 chars) |

**Returns:** `TorrentInstance` with `files`, `progress`, `downloadSpeed`, `numPeers`, `discoveredPeerCount`.

**Throws:** `"Invalid magnet link"`, `"Invalid magnet link format"`, `"Magnet link too long"`, or tracker validation errors.

---

### `addTorrentFile(torrentFile: Uint8Array | Array): Promise<TorrentInstance>`

Add a torrent from raw file bytes.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `torrentFile` | `Uint8Array\|Array` | Raw torrent file bytes (max 10 MB) |

**Returns:** `TorrentInstance`.

**Throws:** `"Invalid torrent file"`, `"Torrent file too large"`, `"Invalid torrent file: not a valid bencoded torrent"`, `"Invalid torrent file: missing required torrent fields"`, or tracker URL validation errors.

---

### `getStats(): Promise<TorrentInstance | null>`

Get current torrent statistics.

**Returns:** `TorrentInstance` or `null` if no active torrent.

---

### `clear(): Promise<void>`

Clear the active torrent and release resources.

---

### `setMaxBufferMB(mb: number): Promise<void>`

Set the maximum buffer size in megabytes.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `mb` | `number` | Buffer size in MB (must be > 0) |

**Throws:** `"Invalid buffer size"`.

---

### `probeAudioTracks(streamUrl: string): Promise<AudioTrackInfo[]>`

Probe a media file for available audio tracks via ffprobe.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `streamUrl` | `string` | Local stream URL (max 5000 chars) |

**Returns:** Array of `AudioTrackInfo` objects with `index`, `label`, `language`, `codecName`, `channels`, `sampleRate`.

**Throws:** `"Invalid stream URL"`.

---

### `createAudioTrackStreamUrl(params): Promise<string>`

Create a stream URL for a specific audio track.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `params.streamUrl` | `string` | Source stream URL |
| `params.trackIndex` | `number` | Audio track index |
| `params.startSeconds` | `number` | Start offset in seconds |

**Returns:** Stream URL string.

---

### `createMultiplexedStreamUrl(params): Promise<string>`

Create a multiplexed audio+video stream URL for perfect sync.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `params.streamUrl` | `string` | Source stream URL |
| `params.audioTrackIndex` | `number` | Audio track index |
| `params.startSeconds` | `number` | Start offset in seconds |

**Returns:** Stream URL string (WebM format).

---

### `createSubtitleStreamUrl(params): Promise<string>`

Create a stream URL for a specific subtitle track.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `params.streamUrl` | `string` | Source stream URL |
| `params.trackIndex` | `number` | Subtitle track index |
| `params.startSeconds` | `number` | Start offset in seconds |

**Returns:** Stream URL string (WebVTT format).

---

### `isFfmpegAvailable(): Promise<boolean>`

Check if ffmpeg is available on the system.

**Returns:** `true` if ffmpeg is detected, `false` otherwise.

---

## `torrsyncElectronWindow` API

Exposed as `window.torrsyncElectronWindow`.

### `onCloseRequest(callback: () => void): void`

Register a handler for window close requests. The main process sends this when the user tries to close the window, allowing the renderer to show a confirmation dialog.

**Parameters:**
| Name | Type | Description |
|------|------|-------------|
| `callback` | `() => void` | Called when close is requested |

---

### `closeConfirmed(): void`

Tell the main process to proceed with closing the application. Destroys all windows and exits.

---

### `closeCancelled(): void`

Tell the main process to cancel the close operation.

---

## Error Handling

All IPC handlers throw errors with descriptive messages. The renderer should catch these and display user-friendly error messages. Common error patterns:

- **Validation errors**: `"Invalid ..."` — input failed validation
- **Size errors**: `"... too large"` — payload exceeded size limit
- **Format errors**: `"not a valid ..."` — data format is incorrect
- **Availability errors**: `"... unavailable"` — required service not available
- **Authorization errors**: `"Unauthorized IPC caller"` — origin validation failed
