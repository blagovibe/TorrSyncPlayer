const shared = require("../torrent-shared.json");

const INFO_HASH_PATTERN = /^(?:urn:)?(?:btih|btmh|sha1):[a-fA-F0-9]{40}$|^(?:urn:)?ed2k:[a-fA-F0-9]{32}$/;
const MAX_MAGNET_LINK_LENGTH = shared.maxMagnetLinkLength;
const MAX_TORRENT_FILE_BYTES = shared.maxTorrentFileBytes;
const MAX_TORRENT_CONNECTIONS = shared.maxTorrentConnections;
const MAX_TORRENT_FILE_COUNT = shared.maxTorrentFileCount;
const MAX_TORRENT_FILENAME_LENGTH = shared.maxTorrentFilenameLength;
const AUDIO_SESSION_TTL_MS = 5 * 60 * 1000;
const SUBTITLE_SESSION_TTL_MS = 5 * 60 * 1000;

const VIDEO_EXTENSIONS = new Set(shared.videoExtensions);

const AUDIO_EXTENSIONS = new Set(shared.audioExtensions);

const BLOCKED_TRACKER_HOSTS = new Set(shared.blockedTrackerHosts);

const ALLOWED_TRACKER_PROTOCOLS = new Set(shared.allowedTrackerProtocols);
const MAX_TRACKER_URL_LENGTH = shared.maxTrackerUrlLength;
const MAX_MAGNET_PARAM_COUNT = shared.maxMagnetParamCount;
const MAX_MAGNET_PARAM_VALUE_LENGTH = shared.maxMagnetParamValueLength;

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

function isValidMagnetLink(magnetLink) {
  if (!magnetLink.startsWith("magnet:?")) return false;
  const queryStart = magnetLink.indexOf("?");
  if (queryStart === -1) return false;
  const params = new URLSearchParams(magnetLink.slice(queryStart + 1));
  const xt = params.get("xt");
  if (!xt) return false;
  return INFO_HASH_PATTERN.test(xt);
}

module.exports = {
  isValidMagnetLink,
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
