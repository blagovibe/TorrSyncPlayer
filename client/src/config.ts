/**
 * Application configuration constants.
 * Centralizes all hardcoded values for easy tuning and environment overrides.
 */

export const TORRENT_CONFIG = {
  maxConnections: 200,
  defaultBufferWindowMB: 50,
  defaultMaxBufferMB: 500,
  prioritizeIntervalMs: 2000,
  bufferWindowStorageKey: "torrsyncplayer.bufferWindowMB",
  maxBufferStorageKey: "torrsyncplayer.maxBufferMB",
} as const;

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

export const P2P_MAX_TORRENT_BYTES = 10 * 1024 * 1024;
export const IPC_MAX_TORRENT_BYTES = 10 * 1024 * 1024;

export const SYNC_CONFIG = {
  defaultToleranceSeconds: 1.5,
  heartbeatIntervalMs: 2000,
  maxLatencyCompensationSeconds: 5,
} satisfies Record<string, number>;

export const STREAM_CONFIG = {
  streamLoadTimeoutMs: 60_000,
  torrentAddTimeoutMs: 60_000,
  torrentDestroyTimeoutMs: 10_000,
  electronForceExitTimeoutMs: 5000,
} as const;

export const BACKEND_CONFIG = {
  audioSessionTtlMs: 5 * 60 * 1000,
  validateStreamUrls: true,
  allowedStreamHosts: ["127.0.0.1", "::1"] as readonly string[],
  subtitleSessionTtlMs: 5 * 60 * 1000,
} as const;

export const UI_CONFIG = {
  hideControlsDelayMs: 3000,
  broadcastDebounceMs: 500,
  maxRoomPasswordLength: 32,
} as const;

export const LOG_CONFIG = {
  defaultLevel: "debug" as const,
} as const;

export function getTrackerUrls(): string[] {
  const env = (import.meta as unknown as { env?: { VITE_WEBTORRENT_TRACKERS?: string } }).env;
  const envTrackers = env?.VITE_WEBTORRENT_TRACKERS;
  if (envTrackers) {
    return envTrackers.split(",").map((s: string) => s.trim()).filter(Boolean);
  }
  return [
    "wss://tracker.btorrent.xyz",
    "wss://tracker.openwebtorrent.com",
    "wss://tracker.webtorrent.dev",
  ];
}

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

export const VIDEO_EXTENSIONS: Readonly<Record<string, true>> = {
  ".mp4": true, ".mkv": true, ".webm": true, ".mov": true,
  ".avi": true, ".m4v": true, ".ts": true, ".ogv": true,
};

export const AUDIO_EXTENSIONS: Readonly<Record<string, true>> = {
  ".mp3": true, ".m4a": true, ".aac": true, ".flac": true,
  ".ogg": true, ".opus": true, ".wav": true, ".oga": true, ".wma": true,
};

export function isVideoExtension(ext: string): boolean {
  return ext in VIDEO_EXTENSIONS;
}

export function isAudioExtension(ext: string): boolean {
  return ext in AUDIO_EXTENSIONS;
}

export function getVideoPreference(ext: string): number {
  return VIDEO_EXTENSION_PREFERENCES[ext] ?? 0;
}
