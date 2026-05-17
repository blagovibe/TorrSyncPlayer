# TorrSyncPlayer — Project Specification

## 1. Overview

**TorrSyncPlayer** is a desktop application for streaming video via WebTorrent with real-time synchronization between multiple clients.

### Key Capabilities
- Stream video from magnet links or .torrent files via WebTorrent
- Synchronized playback across multiple clients (master/slave model)
- PeerJS signaling for P2P connection establishment
- WebRTC data channels for sync messages and torrent source sharing
- Electron-based desktop packaging (AppImage for Linux, NSIS + portable for Windows)

## 2. Architecture

### Components
1. **Electron Desktop App** — client application (React + TypeScript + Electron)
2. **PeerJS Signaling** — cloud-hosted signaling broker for P2P connection setup
3. **WebTorrent** — P2P streaming engine
4. **WebRTC Data Channels** — direct peer-to-peer communication for sync and torrent source sharing

### Interaction Diagram
```
[Host] <---> [PeerJS signaling broker] <---> [Guest]
   |                                            |
   +------------ P2P (WebRTC) -----------------+
   |                                            |
[WebTorrent]                              [WebTorrent]
[Video Playback]                          [Video Playback]
```

### Service Layer
| Service | File | Responsibility |
|---------|------|----------------|
| `P2PService` | `client/src/services/P2PService.ts` | PeerJS connection management, peer discovery, message routing |
| `SyncService` | `client/src/services/SyncService.ts` | Playback state synchronization (play/pause/seek) with latency compensation |
| `TorrentService` | `client/src/services/TorrentService.ts` | Torrent loading (magnet/file), media file discovery, streaming to `<video>` element |

## 3. UI Specification

### Screens
- **HomePage** — create room (host) or join room (guest) with Peer ID
- **RoomPage** — video player + torrent controls (host) or synchronized viewer (guest)

### Color Scheme
- Background: `#1a1a2e` (dark blue)
- Accent: `#e94560` (red)
- Text: `#eaeaea`
- Secondary background: `#16213e`

### UI Components
- Video player with custom controls (play/pause, seek, fullscreen, volume)
- URL bar for magnet links
- File picker for .torrent files
- Media file list (playable files in torrent)
- Audio track selector
- Peer ID display with copy button
- Status bar (connection, download speed, buffer, torrent peers)
- Sync tolerance slider

## 4. Functional Specification

### 4.1 Video Playback
- Supported formats: MP4, WebM, MKV (browser-dependent)
- Progressive streaming via WebTorrent (no full download required)
- Auto-selection of largest video file
- Manual file selection from torrent contents
- Autoplay with fallback (user interaction required by some browsers)

### 4.2 Synchronization
- Master/slave model: one host controls playback, guests follow
- Host broadcasts: play/pause, seek position, media source changes, audio track changes
- Latency compensation: guests apply timestamp corrections with configurable tolerance
- Default sync tolerance: 0.5 seconds
- Guests buffer sync messages until media is ready, then apply

### 4.3 Signaling & P2P
- PeerJS cloud broker for signaling (no self-hosted server required)
- Host generates a 6-character Peer ID (room code)
- Guests connect by entering the host's Peer ID
- WebRTC data channels for all P2P communication after connection
- Host broadcasts room state (torrent source, sync, config) to new peers on connect

### 4.4 Torrent Source Sharing
- Host can load torrents via magnet link or .torrent file
- Torrent source is serialized and sent to guests over WebRTC data channel
- Guests automatically load the same torrent source
- Host can change source (with confirmation dialog)

### 4.5 Audio Track Handling
- Multi-track torrents: host can switch audio tracks
- Fallback audio player for WebView compatibility
- Audio track info probed from media file metadata

### 4.6 Role-Based UI
- **Host**: full torrent controls, magnet input, file picker, media selection, sync tolerance, audio track selector
- **Guest**: read-only view, "Waiting for host" states, automatic sync following

## 5. Technical Stack

- **Frontend**: React 18 + TypeScript + Vite
- **Desktop**: Electron 42 + electron-builder 26
- **Signaling**: PeerJS 1.5 (cloud-hosted broker)
- **P2P Streaming**: WebTorrent 2.6 + WebRTC
- **Testing**: Vitest 2.1
- **Linting**: ESLint 9
- **Styling**: CSS
- **State Management**: Custom Zustand-like store
- **Event Bus**: Custom type-safe EventBus
- **Error Handling**: Result<T,E> pattern
- **Logging**: Structured logging with levels
- **Cleanup**: AbortController-based resource management
- **Retry**: Exponential backoff with jitter
- **Security**: CSP headers, security headers, input validation

## 6. Desktop Packaging

| Platform | Format | Command |
|----------|--------|---------|
| Linux | AppImage | `npm run electron:build:linux` |
| Windows | NSIS installer + Portable EXE | `npm run electron:build:win` |

Output:
- Linux: `client/.electron-app/dist/*.AppImage`
- Windows: `client/.electron-app/dist/*.exe`

## 7. Quality Gates

- `npm run lint` — 0 errors, 0 warnings
- `npm run type-check` — 0 TypeScript errors
- `npm run test` — 39 tests passing (6 test files)
- `npm run build` — successful Vite build

## 8. Acceptance Criteria

- [x] Application launches without errors
- [x] Video plays from magnet link
- [x] Video plays from .torrent file
- [x] Multiple clients synchronize playback
- [x] UI follows color scheme
- [x] Linux AppImage builds successfully
- [x] Windows EXE builds successfully
- [x] Guests cannot see torrent control menu
- [x] Guests receive torrent file from host automatically
- [x] Audio track switching works
- [x] Sync tolerance is adjustable

## 9. Roadmap

1. **Phase 1**: Basic project structure, Electron + React ✅
2. **Phase 2**: WebTorrent integration, video player ✅
3. **Phase 3**: P2P signaling via PeerJS ✅
4. **Phase 4**: Playback synchronization ✅
5. **Phase 5**: UI/UX polish ✅
6. **Phase 6**: Self-hosted signaling option (future)
