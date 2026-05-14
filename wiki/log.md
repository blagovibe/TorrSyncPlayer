# Wiki Log

> Chronological record of all wiki actions. Append-only.
> Format: `## [YYYY-MM-DD] action | subject`
> Actions: ingest, update, query, lint, create, archive, delete

## [2026-05-15] create | Wiki initialized
- Domain: TorrSyncPlayer — P2P видеоплеер с синхронизацией
- Structure created with SCHEMA.md, index.md, log.md
- Created 13 entity pages: P2PService, SyncService, TorrentService, App, VideoPlayer, HomePage, RoomPage, RoomInfo, StatusBar, SyncMessage, SharedTorrentSource, TorrentMediaFile, RoomConfigMessage, format utility
- Created 4 concept pages: WebTorrent Streaming, Playback Synchronization Protocol, PeerJS Signaling, Audio Track Handling
- Ingested 2 raw sources: README.md, SPEC.md

## [2026-05-15] update | Documentation audit and Tauri removal
- Removed all Tauri references from wiki (replaced with Electron)
- Updated SCHEMA.md: desktop tag changed from "Tauri, Electron, Rust" to "Electron"
- Updated peerjs-signaling.md: removed "WebRTC unavailable in Tauri WebView" limitation
- Updated p2p-service.md: removed Tauri limitation
- Replaced raw sources (readme-source.md, spec-source.md) with current versions
- Updated wiki/index.md: reformatted for readability
