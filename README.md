# TorrSyncPlayer

**Watch torrents together with friends — in perfect sync.**

TorrSyncPlayer is a free desktop app that lets you stream video from torrents and magnet links while staying perfectly synchronized with your friends. One person hosts, everyone else watches together.

---

## What It Does

- **Stream video from magnet links or .torrent files** — no need to download the whole file first
- **Watch together in real time** — when the host pauses, seeks, or changes the file, everyone follows
- **Peer-to-peer sharing** — video data is shared directly between viewers, reducing load on the original torrent swarm
- **Simple room system** — create a room, share your ID, and your friends join

---

## Download

| Platform | Format | Status |
|----------|--------|--------|
| **Linux** | AppImage | ✅ Available |
| **Windows** | Portable EXE | ✅ Available |
| **macOS** | — | 🚧 Planned (tracked in backlog) |

> Releases are built automatically. Check the latest CI artifacts for downloadable builds.

---

## Quick Start

### For the Host

1. **Launch** TorrSyncPlayer
2. Click **"Create Room (Host)"**
3. **Copy your Peer ID** (shown after the room is created) and share it with your friends
4. **Paste a magnet link** or **choose a .torrent file**
5. Click **"Load Magnet"** or **"Load File"**
6. Pick a video file from the list (auto-selected by default)
7. Press **Play** — your friends will see the same video at the same time

### For Guests

1. **Launch** TorrSyncPlayer
2. Ask your friend for their **Peer ID**
3. Paste it into **"Connect to Friend"** and click **"Connect"**
4. Wait for the host to load a torrent — the video will appear automatically
5. **Enjoy!** Play/pause/seek are controlled by the host

---

## How It Works (Simple Version)

```
┌────────────┐    PeerJS signaling     ┌────────────┐
│   Host     │ ◄─────────────────────► │   Guest    │
│            │                          │            │
│  ┌──────┐  │   WebRTC data channel   │  ┌──────┐  │
│  │Video │  │ ◄─────────────────────► │  │Video │  │
│  └──────┘  │   (sync + torrent data) │  └──────┘  │
│     ▲      │                          │     ▲      │
│     │      │                          │     │      │
│  WebTorrent│                          │  WebTorrent│
│  (streaming)│                         │  (streaming)│
└────────────┘                          └────────────┘
```

1. **Host** loads a torrent via magnet link or .torrent file
2. **Host's Peer ID** is shared with guests (like a room code)
3. **Guests connect** to the host via PeerJS signaling
4. **Playback sync** (play, pause, seek) is sent over a direct WebRTC connection
5. **Video data** is streamed via WebTorrent — peers can share pieces between themselves
6. **Buffer is RAM-based**: all downloaded chunks are held in memory via `BoundedChunkStore` / `memory-chunk-store`. Nothing is written to disk; old chunks are evicted by LRU policy as playback progresses, capping RAM usage at ~500 MB.

---

## Features

### 🎬 Streaming Playback
- Stream video directly from torrents — no full download required
- Supports MP4, WebM, MKV (browser-dependent)
- Automatic file selection (largest video by default)
- Manual file picker if the torrent contains multiple videos

### 🔄 Perfect Sync
- Host controls playback — guests follow automatically
- Latency compensation keeps everyone in sync even on slower connections
- Adjustable sync tolerance (how much drift is allowed before correcting)

### 🎧 Audio Track Support
- Switch between audio tracks in multi-language torrents
- Fallback audio player for WebView compatibility

### 📊 Status & Info
- Connection status (connected / connecting / disconnected)
- Download speed and torrent progress
- Number of torrent peers
- Buffer status

### 🎨 Clean UI
- Dark theme optimized for video watching
- Minimal, distraction-free player controls

---

## System Requirements

| | Minimum |
|---|---------|
| **OS** | Linux (Ubuntu 20.04+), Windows 10+ |
| **RAM** | 4 GB |
| **Disk** | 200 MB for the app (no disk space needed for streaming — buffer is RAM-based) |
| **Network** | Stable internet connection (both host and guests) |

---

## FAQ

### Do all participants need the torrent file?
**No.** Only the host needs the magnet link or .torrent file. Guests receive everything automatically through the P2P connection.

### Can guests control playback?
**No.** Only the host controls play, pause, seek, and file selection. Guests are synchronized viewers. This ensures everyone sees the same thing at the same time.

### What if the host disconnects?
The session ends for all guests. The host needs to create a new room and re-share their Peer ID.

### Is there a limit on the number of guests?
The app supports multiple simultaneous guests, but performance depends on the host's upload bandwidth and the torrent's availability.

### Why is RAM important for streaming?

TorrSyncPlayer stores the entire torrent buffer in RAM (up to ~500 MB by default) rather than on disk, using `memory-chunk-store` under the hood. This means:

- **No temporary files** are written to your drive — nothing is left behind after you close the app.
- **Video data is ephemeral**: as you play, old chunks are evicted from memory and new ones fill the window, keeping memory usage bounded.
- **Minimum 4 GB RAM** is recommended to comfortably accommodate the Electron runtime (~150–250 MB) plus the streaming buffer (~500 MB peak).

### Does it work without internet?
**No.** Both PeerJS signaling and WebTorrent require an internet connection. However, once connected, video data is shared P2P between participants.

### What video formats are supported?
MP4 (H.264) has the best compatibility. WebM and MKV support depends on the system's codecs.

### Can I use this for audio files?
Yes! The app supports audio files from torrents. The UI will show "Audio file" in the file picker.

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| **"No supported video or audio file found in torrent"** | The torrent may not contain a compatible video/audio file. Try a different torrent. |
| **Video doesn't start playing** | Some browsers block autoplay. Click the Play button in the player manually. |
| **Connection fails / timeout** | Check your internet connection. Make sure the Peer ID is entered correctly (6 characters, uppercase). |
| **Video is buffering slowly** | The torrent may have few seeders. Wait for more pieces to download, or try a more popular torrent. |
| **Audio track not switching** | Not all torrents have multiple audio tracks. Check if the file actually contains multiple tracks. |
| **"Change Source" button missing** | Only the host can change the torrent source. Guests see a read-only view. |

---

## Building from Source

> This section is for developers who want to modify or contribute to the app.

### Prerequisites

- Node.js 18+
- npm 9+

### Setup

```bash
cd client/
npm ci
```

### Development

```bash
npm run dev          # Start Vite dev server
npm run electron:dev # Start Electron with hot reload
```

### Build

```bash
npm run build                    # Web build
npm run electron:build:linux     # Linux AppImage
npm run electron:build:win       # Windows EXE (portable)
```

> **Note:** `electron:build` (without platform suffix) builds for the current platform only.

### Release Process

Releases are created as **drafts** on GitHub and must be manually published. To create a release, push a tag with the `v` prefix (e.g., `v0.1.14`). The CI pipeline will build for Linux and Windows automatically.

### Dependencies

- The `ip` package is overridden to `2.0.1` in `package.json` to address a known security vulnerability in earlier versions.

### Quality Checks

```bash
npm run lint         # ESLint
npm run type-check   # TypeScript
npm run test         # Unit tests (Vitest)
```

---

## Project Structure

```
TorrSyncPlayer/
├── client/                  # Main application (React + Electron)
│   ├── src/
│   │   ├── components/      # React UI components
│   │   ├── services/        # Business logic (P2P, Sync, Torrent)
│   │   └── utils/           # Helpers
│   ├── electron/            # Electron main process
│   └── package.json
└── README.md
```

---

## License

MIT License. See [LICENSE](LICENSE) for details.
