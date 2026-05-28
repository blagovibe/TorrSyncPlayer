/**
 * Application configuration constants.
 * 
 * @deprecated Import from "./config-unified" instead.
 * This file is kept for backward compatibility.
 * 
 * @module config
 */

// Re-export everything from the unified config
export {
  SHARED_TORRENT_LIMITS,
  TORRENT_CONFIG,
  P2P_CONFIG,
  P2P_MAX_TORRENT_BYTES,
  IPC_MAX_TORRENT_BYTES,
  MAX_TORRENT_FILE_BYTES,
  SYNC_CONFIG,
  STREAM_CONFIG,
  UI_CONFIG,
  LOG_CONFIG,
  VIDEO_EXTENSION_PREFERENCES,
  VIDEO_EXTENSIONS,
  AUDIO_EXTENSIONS,
  NATIVE_BROWSER_VIDEO_FORMATS,
  NATIVE_BROWSER_AUDIO_FORMATS,
  isVideoExtension,
  isAudioExtension,
  getVideoPreference,
  needsVideoConversion,
  getTrackerUrls,
  getPeerConnectSources,
} from "./config-unified";
