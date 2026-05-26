import type { SharedTorrentSource } from "../services/types";
import { MAGNET_LINK_PATTERN, MAX_MAGNET_LINK_LENGTH } from "./torrentConstants";
const MAX_TRACKER_URL_LENGTH = 2000;
const MAX_MAGNET_PARAM_COUNT = 20;
const MAX_MAGNET_PARAM_VALUE_LENGTH = 2000;

const BLOCKED_TRACKER_HOSTS = new Set([
  "localhost", "127.0.0.1", "0.0.0.0", "::1", "0000:0000:0000:0000:0000:0000:0000:0001",
  "[::1]", "0:0:0:0:0:0:0:1",
]);

function isBlockedTrackerUrl(trackerUrl: string): boolean {
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

const ALLOWED_TRACKER_PROTOCOLS = new Set(["wss:", "https:"]);

export function isValidMagnetLink(magnetLink: string): boolean {
  const trimmed = magnetLink.trim();
  if (trimmed.length > MAX_MAGNET_LINK_LENGTH) return false;
  if (!MAGNET_LINK_PATTERN.test(trimmed)) return false;
  try {
    const queryStart = trimmed.indexOf("?");
    if (queryStart === -1) return true;
    const params = new URLSearchParams(trimmed.slice(queryStart + 1));
    let paramCount = 0;
    for (const [key, value] of params) {
      paramCount++;
      if (paramCount > MAX_MAGNET_PARAM_COUNT) return false;
      if (value.length > MAX_MAGNET_PARAM_VALUE_LENGTH) return false;
      if (key === "tr" || key.startsWith("tr.")) {
        if (value.length > MAX_TRACKER_URL_LENGTH) return false;
        try {
          const trackerUrl = new URL(value);
          if (!ALLOWED_TRACKER_PROTOCOLS.has(trackerUrl.protocol)) return false;
          if (isBlockedTrackerUrl(value)) return false;
        } catch {
          return false;
        }
      }
      if (value.startsWith("http://") || value.startsWith("https://")) {
        if (value.length > MAX_TRACKER_URL_LENGTH || isBlockedTrackerUrl(value)) return false;
      }
    }
    return true;
  } catch {
    return false;
  }
}

export function hashBytes(bytes: Uint8Array): string {
  let hash = 0x811c9dc5;
  for (const byte of bytes) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash.toString(16).padStart(8, "0");
}

export function createMagnetSource(magnetLink: string): SharedTorrentSource {
  const normalizedMagnetLink = magnetLink.trim();
  if (!isValidMagnetLink(normalizedMagnetLink)) {
    throw new Error("Invalid magnet link format");
  }
  return {
    kind: "magnet",
    magnetLink: normalizedMagnetLink,
    sourceKey: `magnet:${normalizedMagnetLink}`,
  };
}

export function createTorrentFileSource(fileName: string, bytes: Uint8Array): SharedTorrentSource {
  const normalizedFileName = fileName.trim() || "shared.torrent";
  const serialized = new Uint8Array(bytes);
  return {
    kind: "file",
    fileName: normalizedFileName,
    bytes: Array.from(serialized),
    sourceKey: `file:${normalizedFileName}:${bytes.length}:${hashBytes(bytes)}`,
  };
}
