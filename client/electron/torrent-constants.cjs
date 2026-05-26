const MAGNET_LINK_PATTERN = /^magnet:\?xt=urn:(?:btih:[a-fA-F0-9]{40}|btmh:[a-fA-F0-9]{40}|sha1:[a-fA-F0-9]{40}|ed2k:[a-fA-F0-9]{32})(?:&.+)?$/;
const MAX_MAGNET_LINK_LENGTH = 8000;
const MAX_TORRENT_FILE_BYTES = 10 * 1024 * 1024;
const MAX_TORRENT_CONNECTIONS = 200;
const MAX_TORRENT_FILE_COUNT = 10_000;
const MAX_TORRENT_FILENAME_LENGTH = 512;
const AUDIO_SESSION_TTL_MS = 5 * 60 * 1000;
const SUBTITLE_SESSION_TTL_MS = 5 * 60 * 1000;

const VIDEO_EXTENSIONS = new Set([
  ".mp4", ".mkv", ".webm", ".mov", ".avi", ".m4v", ".ts", ".ogv",
]);

const AUDIO_EXTENSIONS = new Set([
  ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".wav", ".oga", ".wma",
]);

const BLOCKED_TRACKER_HOSTS = new Set([
  "localhost", "127.0.0.1", "0.0.0.0", "::1", "0000:0000:0000:0000:0000:0000:0000:0001",
  "[::1]", "0:0:0:0:0:0:0:1",
]);

const ALLOWED_TRACKER_PROTOCOLS = new Set(["wss:", "https:"]);
const MAX_TRACKER_URL_LENGTH = 2000;
const MAX_MAGNET_PARAM_COUNT = 20;
const MAX_MAGNET_PARAM_VALUE_LENGTH = 2000;

function isBlockedTrackerUrl(trackerUrl) {
  try {
    const parsed = new URL(trackerUrl);
    const hostname = parsed.hostname.toLowerCase();
    if (BLOCKED_TRACKER_HOSTS.has(hostname)) return true;
    if (hostname.startsWith("10.") || hostname.startsWith("192.168.") || hostname.startsWith("172.")) {
      const parts = hostname.split(".");
      if (parts[0] === "172") {
        const second = parseInt(parts[1], 10);
        if (second >= 16 && second <= 31) return true;
      }
      return true;
    }
    if (hostname === "[::1]" || hostname === "::1") return true;
    return false;
  } catch {
    return true;
  }
}

function validateMagnetTrackerUrls(magnetLink) {
  const queryStart = magnetLink.indexOf("?");
  if (queryStart === -1) return;
  const params = new URLSearchParams(magnetLink.slice(queryStart + 1));
  let paramCount = 0;
  for (const [key, value] of params) {
    paramCount++;
    if (paramCount > MAX_MAGNET_PARAM_COUNT) throw new Error("Too many magnet link parameters");
    if (value.length > MAX_MAGNET_PARAM_VALUE_LENGTH) throw new Error("Magnet link parameter value too long");
    if (key === "tr" || key.startsWith("tr.")) {
      if (value.length > MAX_TRACKER_URL_LENGTH) throw new Error("Tracker URL too long");
      const trackerUrl = new URL(value);
      if (!ALLOWED_TRACKER_PROTOCOLS.has(trackerUrl.protocol)) {
        throw new Error(`Tracker uses disallowed protocol: ${trackerUrl.protocol}`);
      }
      if (isBlockedTrackerUrl(value)) {
        throw new Error(`Tracker URL points to blocked address: ${value}`);
      }
    }
    if (value.startsWith("http://") || value.startsWith("https://")) {
      if (value.length > MAX_TRACKER_URL_LENGTH || isBlockedTrackerUrl(value)) {
        throw new Error(`URL points to blocked address: ${value}`);
      }
    }
  }
}

module.exports = {
  MAGNET_LINK_PATTERN,
  MAX_MAGNET_LINK_LENGTH,
  MAX_TORRENT_FILE_BYTES,
  MAX_TORRENT_CONNECTIONS,
  MAX_TORRENT_FILE_COUNT,
  MAX_TORRENT_FILENAME_LENGTH,
  AUDIO_SESSION_TTL_MS,
  SUBTITLE_SESSION_TTL_MS,
  VIDEO_EXTENSIONS,
  AUDIO_EXTENSIONS,
  validateMagnetTrackerUrls,
};
