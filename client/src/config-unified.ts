/**
 * Unified configuration for TorrSyncPlayer.
 * 
 * This is the single source of truth for all configuration values.
 * Import from this file instead of config.ts, config-shared.ts, or torrent-shared.json.
 * 
 * @module config-unified
 */

import sharedConfig from "../torrent-shared.json";

// =============================================================================
// Re-export shared config for backward compatibility
// =============================================================================

/**
 * Shared torrent limits loaded from torrent-shared.json.
 * @deprecated Import specific values from this module instead.
 */
export const SHARED_TORRENT_LIMITS = {
  maxTorrentFileBytes: sharedConfig.maxTorrentFileBytes,
  maxMagnetLinkLength: sharedConfig.maxMagnetLinkLength,
  maxTorrentConnections: sharedConfig.maxTorrentConnections,
  maxTorrentFileCount: sharedConfig.maxTorrentFileCount,
  maxTorrentFilenameLength: sharedConfig.maxTorrentFilenameLength,
  videoExtensions: sharedConfig.videoExtensions as readonly string[],
  audioExtensions: sharedConfig.audioExtensions as readonly string[],
  nativeBrowserVideoFormats: sharedConfig.nativeBrowserVideoFormats as readonly string[],
  nativeBrowserAudioFormats: sharedConfig.nativeBrowserAudioFormats as readonly string[],
  blockedTrackerHosts: sharedConfig.blockedTrackerHosts,
  allowedTrackerProtocols: sharedConfig.allowedTrackerProtocols,
  maxTrackerUrlLength: sharedConfig.maxTrackerUrlLength,
  maxMagnetParamCount: sharedConfig.maxMagnetParamCount,
  maxMagnetParamValueLength: sharedConfig.maxMagnetParamValueLength,
} as const;

// =============================================================================
// Torrent Configuration
// =============================================================================

/**
 * WebTorrent client configuration.
 */
export const TORRENT_CONFIG = {
  maxConnections: SHARED_TORRENT_LIMITS.maxTorrentConnections,
  defaultBufferWindowMB: 50,
  defaultMaxBufferMB: 500,
  prioritizeIntervalMs: 2000,
  bufferWindowStorageKey: "torrsyncplayer.bufferWindowMB",
  maxBufferStorageKey: "torrsyncplayer.maxBufferMB",
} as const;

// =============================================================================
// P2P Configuration
// =============================================================================

/**
 * Peer-to-peer connection configuration for PeerJS.
 */
export const P2P_CONFIG = {
  peerIdLength: 6,
  peerIdChars: "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
  defaultHost: "0.peerjs.com",
  defaultPort: 443,
  defaultPath: "/",
  hostPeerPrefix: "torrsync-",
  connectionTimeoutMs: 30_000,
  initTimeoutMs: 15_000,
  connectRetryAttempts: 3,
  connectRetryBaseDelayMs: 1000,
} as const;

/** Maximum torrent file size in bytes for P2P transfers. */
export const P2P_MAX_TORRENT_BYTES = SHARED_TORRENT_LIMITS.maxTorrentFileBytes;

/** Maximum torrent file size in bytes for IPC transfers. */
export const IPC_MAX_TORRENT_BYTES = SHARED_TORRENT_LIMITS.maxTorrentFileBytes;

/** Maximum torrent file size in bytes (general). */
export const MAX_TORRENT_FILE_BYTES = SHARED_TORRENT_LIMITS.maxTorrentFileBytes;

// =============================================================================
// Sync Configuration
// =============================================================================

/**
 * Playback synchronization configuration.
 */
export const SYNC_CONFIG = {
  defaultToleranceSeconds: 1.5,
  heartbeatIntervalMs: 2000,
  maxLatencyCompensationSeconds: 5,
} satisfies Record<string, number>;

// =============================================================================
// Stream Configuration
// =============================================================================

/**
 * Stream and timeout configuration.
 */
export const STREAM_CONFIG = {
  streamLoadTimeoutMs: 60_000,
  torrentAddTimeoutMs: 60_000,
  torrentDestroyTimeoutMs: 10_000,
  electronForceExitTimeoutMs: 5000,
  torrentQueueTimeoutMs: 120_000,
} as const;

// =============================================================================
// UI Configuration
// =============================================================================

/**
 * User interface configuration.
 */
export const UI_CONFIG = {
  hideControlsDelayMs: 3000,
  broadcastDebounceMs: 500,
  maxChatMessages: 500,
} as const;

// =============================================================================
// Log Configuration
// =============================================================================

/**
 * Logging configuration.
 */
export const LOG_CONFIG = {
  defaultLevel: "debug" as const,
} as const;

// =============================================================================
// Video/Audio Extension Helpers
// =============================================================================

/**
 * Video file extension preferences for sorting.
 * Higher values indicate higher priority.
 */
export const VIDEO_EXTENSION_PREFERENCES: Readonly<Record<string, number>> = {
  ".mp4": 4,
  ".m4v": 4,
  ".webm": 3,
  ".mov": 2,
  ".ogv": 2,
  ".ts": 1,
  ".mkv": -1,
  ".avi": -1,
};

/** Set of video file extensions. */
export const VIDEO_EXTENSIONS: Readonly<Record<string, true>> = Object.fromEntries(
  SHARED_TORRENT_LIMITS.videoExtensions.map((ext) => [ext, true as const]),
) as Readonly<Record<string, true>>;

/** Set of audio file extensions. */
export const AUDIO_EXTENSIONS: Readonly<Record<string, true>> = Object.fromEntries(
  SHARED_TORRENT_LIMITS.audioExtensions.map((ext) => [ext, true as const]),
) as Readonly<Record<string, true>>;

/** Set of natively supported browser video formats. */
export const NATIVE_BROWSER_VIDEO_FORMATS = new Set(SHARED_TORRENT_LIMITS.nativeBrowserVideoFormats);

/** Set of natively supported browser audio formats. */
export const NATIVE_BROWSER_AUDIO_FORMATS = new Set(SHARED_TORRENT_LIMITS.nativeBrowserAudioFormats);

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Check if a file extension is a video format.
 * @param ext - The file extension (e.g., ".mp4").
 * @returns True if the extension is a video format.
 */
export function isVideoExtension(ext: string): boolean {
  return ext in VIDEO_EXTENSIONS;
}

/**
 * Check if a file extension is an audio format.
 * @param ext - The file extension (e.g., ".mp3").
 * @returns True if the extension is an audio format.
 */
export function isAudioExtension(ext: string): boolean {
  return ext in AUDIO_EXTENSIONS;
}

/**
 * Get the preference score for a video file extension.
 * @param ext - The file extension (e.g., ".mp4").
 * @returns The preference score (higher is better).
 */
export function getVideoPreference(ext: string): number {
  return VIDEO_EXTENSION_PREFERENCES[ext] ?? 0;
}

/**
 * Check if a video format needs conversion for browser playback.
 * @param ext - The file extension (e.g., ".mkv").
 * @returns True if the format is not natively supported by browsers.
 */
export function needsVideoConversion(ext: string): boolean {
  return isVideoExtension(ext) && !NATIVE_BROWSER_VIDEO_FORMATS.has(ext);
}

/**
 * Get the list of WebTorrent tracker URLs.
 * Uses VITE_WEBTORRENT_TRACKERS environment variable if set, otherwise defaults to public trackers.
 * @returns Array of tracker WebSocket URLs.
 */
export function getTrackerUrls(): string[] {
  const env = import.meta.env;
  const envTrackers = env.VITE_WEBTORRENT_TRACKERS;
  if (envTrackers) {
    return envTrackers.split(",").map((s: string) => s.trim()).filter(Boolean);
  }
  return [
    "wss://tracker.btorrent.xyz",
    "wss://tracker.openwebtorrent.com",
    "wss://tracker.webtorrent.dev",
  ];
}

/**
 * Get the list of PeerJS connection sources for CSP configuration.
 * @returns Array of WebSocket source URLs.
 */
export function getPeerConnectSources(): string[] {
  const env = import.meta.env;
  const host = env.VITE_PEERJS_HOST?.trim() || P2P_CONFIG.defaultHost;
  const secure = host === P2P_CONFIG.defaultHost || env.VITE_PEERJS_SECURE === "true";
  const port = env.VITE_PEERJS_PORT ? Number(env.VITE_PEERJS_PORT) : P2P_CONFIG.defaultPort;
  const sources = [`wss://*.openwebtorrent.com`, `wss://*.webtorrent.dev`, `wss://*.btorrent.xyz`];
  if (host === P2P_CONFIG.defaultHost) {
    sources.push(`wss://0.peerjs.com`);
  } else if (host) {
    if (secure || port === 443) {
      sources.push(`wss://${host}`);
    } else {
      sources.push(`ws://${host}`);
    }
  }
  return sources;
}
