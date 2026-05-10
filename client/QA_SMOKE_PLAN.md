# QA Audit and Smoke Validation Plan

Date: 2026-05-10

## Current Validation Baseline

Executed in `client/`:

- `npm run lint` -> pass with 1 warning:
  - `src/components/VideoPlayer.tsx:81` (`react-hooks/exhaustive-deps`)
- `npm run type-check` -> pass
- `npm run test` -> pass (`4` files, `16` tests)
- `timeout 180 npm run build` -> timed out (exit `124`)

Rust/Tauri validation:

- `cargo check` -> unavailable in current environment (`cargo: command not found`)
- `cargo test` -> unavailable in current environment (`cargo: command not found`)
- No Rust unit tests detected in `client/src-tauri/src` (`#[test]`/`#[cfg(test)]` absent)

## Testability Assessment

Strengths:

- Service-layer logic is unit-tested (`SyncService`, `SignalingService`, `PeerService`, `TorrentService`)
- CI already gates lint, type-check, and unit tests before packaging

Gaps affecting smoke confidence:

- No component/UI tests for `HomePage`, `RoomPage`, `VideoPlayer`
- No integration/e2e test for host-guest pairing and sync behavior
- No Rust-side tests for `sync_engine` or `settings` persistence
- Production web build currently unbounded/slow from smoke perspective (does not complete within 180s here)
- Desktop runtime smoke (Tauri launch + app readiness) not automated in CI

## Recommended Smoke Plan

### 1) Fast pre-merge smoke (required)

Run on every PR:

1. `npm ci`
2. `npm run lint`
3. `npm run type-check`
4. `npm run test`
5. `timeout 300 npm run build` (treat timeout as failure)

Goal: catch obvious regressions in under ~10 minutes.

### 2) Desktop build smoke (required for release and main branch)

Run in CI runners with Rust toolchain:

1. `cargo check` in `client/src-tauri`
2. `cargo test` in `client/src-tauri`
3. `npx tauri build --bundles appimage` (Linux)
4. `npx tauri build --bundles app` (Windows)

Goal: verify desktop layer and packaging are healthy.

### 3) Runtime functional smoke (manual until e2e exists)

Per release candidate:

1. Launch host app, create room, copy/share peer ID
2. Launch guest app, join by peer ID
3. Verify connect/disconnect status and peer list updates
4. Load valid magnet, verify progress/speed updates and video starts
5. Validate sync:
   - master play/pause reflected on guest
   - master seek reflected on guest
6. Negative checks:
   - invalid join code shows error
   - magnet with no video file shows user-facing error
7. Leave room on both clients, verify clean return to home state

Pass criteria: all steps succeed without crash/hang and with expected UI status changes.

## Next Highest-Value Improvements

1. Add Rust unit tests for `sync_engine` latency compensation and `settings` load/save edge cases
2. Add component tests for room join form validation and video controls
3. Add a minimal two-client smoke harness (Playwright or scripted WebRTC mock path)
4. Investigate `npm run build` duration/stall risk and enforce a CI timeout gate
