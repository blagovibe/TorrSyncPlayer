import type { SharedTorrentSource } from "../services/types";

const MAGNET_LINK_PATTERN = /^magnet:\?xt=urn:(?:btih:[a-fA-F0-9]{40}|btmh:[a-fA-F0-9]{40}|sha1:[a-fA-F0-9]{40}|ed2k:[a-fA-F0-9]{32})(?:&.+)?$/;
const MAX_MAGNET_LINK_LENGTH = 8000;
const MAX_TRACKER_URL_LENGTH = 2000;

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

export function isValidMagnetLink(magnetLink: string): boolean {
  const trimmed = magnetLink.trim();
  if (trimmed.length > MAX_MAGNET_LINK_LENGTH) return false;
  if (!MAGNET_LINK_PATTERN.test(trimmed)) return false;
  try {
    const queryStart = trimmed.indexOf("?");
    if (queryStart === -1) return true;
    const params = new URLSearchParams(trimmed.slice(queryStart + 1));
    for (const [, value] of params) {
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
  return {
    kind: "file",
    fileName: normalizedFileName,
    bytes: Array.from(bytes),
    sourceKey: `file:${normalizedFileName}:${bytes.length}:${hashBytes(bytes)}`,
  };
}
