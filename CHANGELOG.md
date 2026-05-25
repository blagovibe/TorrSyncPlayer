# Changelog

All notable changes to TorrSyncPlayer will be documented in this file.

## [Unreleased]

### Security
- Added input validation to outbound P2P messages (`sendTorrentSource`, `sendSync`, `sendRoomConfig`) to prevent sending invalid data to peers
- Reduced RTT upper bound from 30s to 5s to prevent misleading connection quality readings
- Clamped `syncToleranceSeconds` at send boundary to prevent propagating invalid tolerance values to guests

### Bug Fixes
- Fixed P2PService reconnect timer leak — reconnect timeouts are now properly cleared on disconnect
- Fixed App.tsx double-cleanup on unmount — added idempotency guard to prevent errors from React strict mode double-fire
- Fixed electronBackend stale cache — backend is now re-checked from `window.torrsyncElectronTorrent` on each access instead of caching permanently
- Fixed SyncService heartbeat suppression flag race — removed unconditional flag clearing from heartbeat that could cause duplicate sync events

### Architecture
- Extracted torrent utility functions (`hashBytes`, `createMagnetSource`, `createTorrentFileSource`) to `utils/torrent.ts`
- Created `useRoomState` hook to manage room state refs
- Created shared `electron-api.d.ts` type definitions for the Electron API surface
- Consolidated shared test utilities (`createTorrent`, `setupElectronBackendCleanup`) into `test-utils.ts`

### Build & CI
- Added `license: "MIT"` field to both `package.json` files
- Added `npm audit` step to CI for vulnerability scanning
- Added `type-check` step to Windows CI builds
