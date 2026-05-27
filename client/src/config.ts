/**
 * Application configuration constants.
 * Centralizes all hardcoded values for easy tuning and environment overrides.
 */

import { SHARED_TORRENT_LIMITS } from "./config-shared";

export const TORRENT_CONFIG = {
  maxConnections: SHARED_TORRENT_LIMITS.maxTorrentConnections,
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

export const P2P_MAX_TORRENT_BYTES = SHARED_TORRENT_LIMITS.maxTorrentFileBytes;
export const IPC_MAX_TORRENT_BYTES = SHARED_TORRENT_LIMITS.maxTorrentFileBytes;
export const MAX_TORRENT_FILE_BYTES = SHARED_TORRENT_LIMITS.maxTorrentFileBytes;

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
  torrentQueueTimeoutMs: 120_000,
} as const;

export const UI_CONFIG = {
  hideControlsDelayMs: 3000,
  broadcastDebounceMs: 500,
  maxRoomPasswordLength: 32,
  maxChatMessages: 500,
} as const;

export const LOG_CONFIG = {
  defaultLevel: "debug" as const,
} as const;

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

export const VIDEO_EXTENSIONS: Readonly<Record<string, true>> = Object.fromEntries(
  SHARED_TORRENT_LIMITS.videoExtensions.map((ext) => [ext, true as const]),
) as Readonly<Record<string, true>>;

export const AUDIO_EXTENSIONS: Readonly<Record<string, true>> = Object.fromEntries(
  SHARED_TORRENT_LIMITS.audioExtensions.map((ext) => [ext, true as const]),
) as Readonly<Record<string, true>>;

export function isVideoExtension(ext: string): boolean {
  return ext in VIDEO_EXTENSIONS;
}

export function isAudioExtension(ext: string): boolean {
  return ext in AUDIO_EXTENSIONS;
}

export function getVideoPreference(ext: string): number {
  return VIDEO_EXTENSION_PREFERENCES[ext] ?? 0;
}

const NATIVE_BROWSER_VIDEO_FORMATS = new Set([".mp4", ".webm", ".ogv", ".mov", ".m4v", ".ts"]);

export function needsVideoConversion(ext: string): boolean {
  return isVideoExtension(ext) && !NATIVE_BROWSER_VIDEO_FORMATS.has(ext);
}
