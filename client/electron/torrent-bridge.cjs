const { electronLogger } = require("./electron-logger.cjs");
const { BoundedChunkStore } = require("./bounded-chunk-store.cjs");
const { checkFfmpegAvailable } = require("./ffmpeg-pipeline.cjs");
const { AudioSessionManager, validateLocalStreamUrl } = require("./audio-session-manager.cjs");

const {
  MAX_TORRENT_CONNECTIONS,
  AUDIO_SESSION_TTL_MS,
  SUBTITLE_SESSION_TTL_MS,
  VIDEO_EXTENSIONS,
  AUDIO_EXTENSIONS,
  MAX_TORRENT_FILE_COUNT,
  MAX_TORRENT_FILENAME_LENGTH,
} = require("./torrent-constants.cjs");

const DEFAULT_MAX_BUFFER_MB = 500;
const TORRENT_SERVER_HOST = "127.0.0.1";
const TORRENT_SERVER_PORT = 0;

function normalizePeerId(peerId) {
  if (peerId == null) return null;
  const normalized = String(peerId).trim();
  return normalized.length > 0 ? normalized : null;
}

function getFileExtension(name) {
  const normalized = String(name || "").trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) return "";
  return normalized.slice(lastDot);
}

function getMediaKind(extension) {
  if (VIDEO_EXTENSIONS.has(extension)) return "video";
  if (AUDIO_EXTENSIONS.has(extension)) return "audio";
  return null;
}

function validateTorrentFiles(files) {
  if (!Array.isArray(files)) return;
  if (files.length > MAX_TORRENT_FILE_COUNT) {
    throw new Error(`Torrent contains too many files (${files.length}). Maximum allowed is ${MAX_TORRENT_FILE_COUNT}.`);
  }
  for (const file of files) {
    if (typeof file?.name === "string" && file.name.length > MAX_TORRENT_FILENAME_LENGTH) {
      throw new Error(`Torrent file name exceeds maximum length (${file.name.length} > ${MAX_TORRENT_FILENAME_LENGTH}): ${file.name.slice(0, 64)}…`);
    }
  }
}

function formatTorrentFile(file, index, streamBaseUrl) {
  const extension = getFileExtension(file.name);
  const kind = getMediaKind(extension);
  if (!kind) return null;

  const streamPath = typeof file.streamURL === "string" ? file.streamURL : undefined;
  let streamUrl;
  if (streamBaseUrl && streamPath) {
    try {
      streamUrl = new URL(streamPath, streamBaseUrl).href;
    } catch {
      streamUrl = streamPath;
    }
  } else {
    streamUrl = streamPath;
  }

  return {
    index, name: file.name, length: file.length ?? 0, kind, extension,
    progress: typeof file.progress === "number" ? file.progress : 0,
    streamUrl,
  };
}

function formatTorrentSnapshot(torrent, discoveredPeerCount = 0, streamBaseUrl) {
  validateTorrentFiles(torrent.files);
  const files = torrent.files
    .map((file, index) => formatTorrentFile(file, index, streamBaseUrl))
    .filter(Boolean)
    .sort((left, right) => {
      if (left.kind !== right.kind) return left.kind === "video" ? -1 : 1;
      if (right.length !== left.length) return right.length - left.length;
      return left.name.localeCompare(right.name);
    });

  return {
    files,
    progress: torrent.progress ?? 0,
    downloadSpeed: torrent.downloadSpeed ?? 0,
    numPeers: torrent.numPeers ?? 0,
    discoveredPeerCount,
  };
}

function normalizeTorrentSource(torrentSource) {
  if (torrentSource instanceof Uint8Array) return torrentSource;
  if (Array.isArray(torrentSource)) return Uint8Array.from(torrentSource);
  throw new Error("Invalid torrent source type: expected Uint8Array or Array");
}

class TorrentBridge {
  static async checkFfmpegAvailable() {
    return checkFfmpegAvailable();
  }

  constructor() {
    this.clientPromise = null;
    this.client = null;
    this.activeTorrent = null;
    this.discoveredPeerIds = new Set();
    this.streamBaseUrl = null;
    this.serverPromise = null;
    this.maxBufferBytes = DEFAULT_MAX_BUFFER_MB * 1024 * 1024;
    this.audioSessionManager = new AudioSessionManager();
  }

  setMaxBufferMB(mb) {
    this.maxBufferBytes = Math.max(1, mb) * 1024 * 1024;
  }

  setStreamBaseUrl(url) {
    this.audioSessionManager.setBaseUrl(url);
  }

  async addMagnet(magnetLink) {
    const { isValidMagnetLink, MAX_MAGNET_LINK_LENGTH: MAX_MAGNET_LEN } = require("./torrent-constants.cjs");
    if (typeof magnetLink !== "string" || magnetLink.length > MAX_MAGNET_LEN) {
      throw new Error("Invalid magnet link: too long or not a string");
    }
    if (!isValidMagnetLink(magnetLink)) {
      throw new Error("Invalid magnet link format");
    }
    const { validateMagnetTrackerUrls } = require("./torrent-constants.cjs");
    validateMagnetTrackerUrls(magnetLink);
    return this.addSource(magnetLink);
  }

  async addTorrentFile(torrentFile) {
    return this.addSource(normalizeTorrentSource(torrentFile));
  }

  async getStats() {
    if (!this.activeTorrent) return null;
    return formatTorrentSnapshot(this.activeTorrent, this.discoveredPeerIds.size, this.streamBaseUrl);
  }

  async clear() {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    this.discoveredPeerIds.clear();
    this.audioSessionManager.clearAll();

    if (!torrent || typeof torrent.destroy !== "function") return;

    await new Promise((resolve) => {
      const timeout = setTimeout(() => { resolve(); }, 10_000);
      timeout.unref?.();
      try {
        torrent.destroy(() => { clearTimeout(timeout); resolve(); });
      } catch {
        clearTimeout(timeout);
        resolve();
      }
    });
  }

  async destroy() {
    const client = this.client;
    this.activeTorrent = null;
    this.discoveredPeerIds.clear();
    this.streamBaseUrl = null;
    this.serverPromise = null;
    this.client = null;
    this.clientPromise = null;
    this.audioSessionManager.destroy();

    if (client && !client.destroyed && typeof client._server === "object" && client._server !== null) {
      const server = client._server;
      if (typeof server.close === "function") {
        try { server.close(); } catch { /* already closed */ }
      }
    }

    if (!client || client.destroyed || typeof client.destroy !== "function") return;

    await new Promise((resolve) => { client.destroy(() => resolve()); });
  }

  async addSource(torrentSource) {
    const client = await this.getClient();
    await this.ensureTorrentServer(client);
    await this.clear();

    const MAX_PEER_IDS = 50_000;

    const torrent = await new Promise((resolve, reject) => {
      let settled = false;
      const settleResolve = (value) => {
        if (settled) return;
        settled = true;
        clearTimeout(addTimeout);
        resolve(value);
      };
      const settleReject = (error) => {
        if (settled) return;
        settled = true;
        clearTimeout(addTimeout);
        reject(error);
      };

      let addedTorrent;
      try {
        addedTorrent = client.add(torrentSource, { store: BoundedChunkStore, storeOpts: { maxBytes: this.maxBufferBytes } }, (readyTorrent) => {
          settleResolve(readyTorrent);
        });
      } catch (error) {
        settleReject(error instanceof Error ? error : new Error(String(error)));
        return;
      }

      if (!addedTorrent) {
        settleReject(new Error("Failed to add torrent: client.add returned null"));
        return;
      }

      const addTimeout = setTimeout(() => {
        settleReject(new Error("Torrent addition timed out after 60 seconds"));
      }, 60_000);

      addedTorrent.on("peer", (peerId) => {
        const normalized = normalizePeerId(peerId);
        if (normalized && this.discoveredPeerIds.size < MAX_PEER_IDS) {
          this.discoveredPeerIds.add(normalized);
        }
      });

      addedTorrent.on("error", (error) => {
        settleReject(error instanceof Error ? error : new Error(String(error)));
      });
    });
    this.activeTorrent = torrent;
    return formatTorrentSnapshot(torrent, this.discoveredPeerIds.size, this.streamBaseUrl);
  }

  async probeAudioTracks(streamUrl) {
    return this.audioSessionManager.probeAudioTracks(streamUrl);
  }

  async probeSubtitles(streamUrl) {
    return this.audioSessionManager.probeSubtitles(streamUrl);
  }

  async createSubtitleStreamUrl({ streamUrl, trackIndex, startSeconds }) {
    return this.audioSessionManager.createSubtitleStreamUrl({
      streamUrl, trackIndex, startSeconds, ttlMs: SUBTITLE_SESSION_TTL_MS,
    });
  }

  async createAudioTrackStreamUrl({ streamUrl, trackIndex, startSeconds }) {
    return this.audioSessionManager.createAudioTrackStreamUrl({
      streamUrl, trackIndex, startSeconds, ttlMs: AUDIO_SESSION_TTL_MS,
    });
  }

  async createMultiplexedStreamUrl({ streamUrl, audioTrackIndex, startSeconds }) {
    return this.audioSessionManager.createMultiplexedStreamUrl({
      streamUrl, audioTrackIndex, startSeconds, ttlMs: AUDIO_SESSION_TTL_MS,
    });
  }

  async getClient() {
    if (this.client) return this.client;

    if (!this.clientPromise) {
      this.clientPromise = Promise.race([
        import("webtorrent").then(({ default: WebTorrent }) => {
          this.client = new WebTorrent({ maxConns: MAX_TORRENT_CONNECTIONS, sequential: true });
          return this.client;
        }),
        new Promise((_, reject) => {
          const id = setTimeout(() => {
            this.clientPromise = null;
            reject(new Error("WebTorrent client import timed out after 10 seconds"));
          }, 10_000);
          id.unref?.();
        }),
      ]);
    }

    return this.clientPromise;
  }

  async ensureTorrentServer(client) {
    if (this.streamBaseUrl) return this.streamBaseUrl;
    if (this.serverPromise) return this.serverPromise;

    const existingServer = client?._server;
    if (existingServer && typeof existingServer.address === "function") {
      const address = existingServer.address();
      if (address && typeof address !== "string") {
        this.streamBaseUrl = `http://${TORRENT_SERVER_HOST}:${address.port}`;
        return this.streamBaseUrl;
      }
    }

    if (!client || typeof client.createServer !== "function") {
      throw new Error("WebTorrent client server API is unavailable");
    }

    this.serverPromise = (async () => {
      const server = client.createServer();

      await new Promise((resolve, reject) => {
        const underlyingServer = server?.server;
        const handleError = (error) => {
          if (underlyingServer && typeof underlyingServer.removeListener === "function") {
            underlyingServer.removeListener("error", handleError);
          }
          reject(error instanceof Error ? error : new Error(String(error)));
        };

        if (underlyingServer && typeof underlyingServer.once === "function") {
          underlyingServer.once("error", handleError);
        }

        try {
          server.listen(TORRENT_SERVER_PORT, TORRENT_SERVER_HOST, () => {
            if (underlyingServer && typeof underlyingServer.removeListener === "function") {
              underlyingServer.removeListener("error", handleError);
            }
            resolve(undefined);
          });
        } catch (error) {
          if (underlyingServer && typeof underlyingServer.removeListener === "function") {
            underlyingServer.removeListener("error", handleError);
          }
          handleError(error);
        }
      });

      const address = server.address();
      if (!address || typeof address === "string") {
        throw new Error("Unable to determine torrent server address");
      }

      this.streamBaseUrl = `http://${TORRENT_SERVER_HOST}:${address.port}`;
      return this.streamBaseUrl;
    })();

    return this.serverPromise;
  }

  async handleAudioRequest(request, response) {
    return this.audioSessionManager.handleAudioRequest(request, response, this.streamBaseUrl);
  }
}

module.exports = { TorrentBridge, formatTorrentSnapshot, validateLocalStreamUrl };
