const VIDEO_EXTENSIONS = new Set([
  ".mp4",
  ".mkv",
  ".webm",
  ".mov",
  ".avi",
  ".m4v",
  ".ts",
  ".ogv",
]);

const AUDIO_EXTENSIONS = new Set([
  ".mp3",
  ".m4a",
  ".aac",
  ".flac",
  ".ogg",
  ".opus",
  ".wav",
  ".oga",
  ".wma",
]);
const MAX_TORRENT_CONNECTIONS = 200;
const TORRENT_SERVER_HOST = "127.0.0.1";
const TORRENT_SERVER_PORT = 0;

function normalizePeerId(peerId) {
  if (peerId == null) {
    return null;
  }

  const normalized = String(peerId).trim();
  return normalized.length > 0 ? normalized : null;
}

function getFileExtension(name) {
  const normalized = String(name || "").trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) {
    return "";
  }
  return normalized.slice(lastDot);
}

function getMediaKind(extension) {
  if (VIDEO_EXTENSIONS.has(extension)) {
    return "video";
  }
  if (AUDIO_EXTENSIONS.has(extension)) {
    return "audio";
  }
  return null;
}

function formatTorrentFile(file, index, streamBaseUrl) {
  const extension = getFileExtension(file.name);
  const kind = getMediaKind(extension);
  if (!kind) {
    return null;
  }

  const streamPath = typeof file.streamURL === "string" ? file.streamURL : undefined;

  return {
    index,
    name: file.name,
    length: file.length ?? 0,
    kind,
    extension,
    progress: typeof file.progress === "number" ? file.progress : 0,
    streamUrl: streamBaseUrl && streamPath ? new URL(streamPath, streamBaseUrl).href : streamPath,
  };
}

function formatTorrentSnapshot(torrent, discoveredPeerCount = 0, streamBaseUrl) {
  const files = torrent.files
    .map((file, index) => formatTorrentFile(file, index, streamBaseUrl))
    .filter(Boolean)
    .sort((left, right) => {
      if (left.kind !== right.kind) {
        return left.kind === "video" ? -1 : 1;
      }
      if (right.length !== left.length) {
        return right.length - left.length;
      }
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
  if (torrentSource instanceof Uint8Array) {
    return torrentSource;
  }
  if (Array.isArray(torrentSource)) {
    return Uint8Array.from(torrentSource);
  }
  return torrentSource;
}

class TorrentBridge {
  constructor() {
    this.clientPromise = null;
    this.client = null;
    this.activeTorrent = null;
    this.discoveredPeerIds = new Set();
    this.streamBaseUrl = null;
    this.serverPromise = null;
  }

  async addMagnet(magnetLink) {
    return this.addSource(magnetLink);
  }

  async addTorrentFile(torrentFile) {
    return this.addSource(normalizeTorrentSource(torrentFile));
  }

  async getStats() {
    if (!this.activeTorrent) {
      return null;
    }

    return formatTorrentSnapshot(this.activeTorrent, this.discoveredPeerIds.size, this.streamBaseUrl);
  }

  async clear() {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    this.discoveredPeerIds.clear();

    if (!torrent || typeof torrent.destroy !== "function") {
      return;
    }

    await new Promise((resolve) => {
      torrent.destroy(() => resolve());
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

    if (!client || client.destroyed || typeof client.destroy !== "function") {
      return;
    }

    await new Promise((resolve) => {
      client.destroy(() => resolve());
    });
  }

  async addSource(torrentSource) {
    const client = await this.getClient();
    await this.ensureTorrentServer(client);
    await this.clear();
    const torrent = await new Promise((resolve, reject) => {
      const addedTorrent = client.add(torrentSource, (readyTorrent) => {
        resolve(readyTorrent);
      });

      addedTorrent.on("peer", (peerId) => {
        const normalizedPeerId = normalizePeerId(peerId);
        if (normalizedPeerId) {
          this.discoveredPeerIds.add(normalizedPeerId);
        }
      });

      addedTorrent.on("error", (error) => {
        reject(error instanceof Error ? error : new Error(String(error)));
      });
    });
    this.activeTorrent = torrent;
    return formatTorrentSnapshot(torrent, this.discoveredPeerIds.size, this.streamBaseUrl);
  }

  async getClient() {
    if (this.client) {
      return this.client;
    }

    if (!this.clientPromise) {
      this.clientPromise = import("webtorrent").then(({ default: WebTorrent }) => {
        this.client = new WebTorrent({
          maxConns: MAX_TORRENT_CONNECTIONS,
        });
        return this.client;
      });
    }

    return this.clientPromise;
  }

  async ensureTorrentServer(client) {
    if (this.streamBaseUrl) {
      return this.streamBaseUrl;
    }

    if (this.serverPromise) {
      return this.serverPromise;
    }

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
      const server = client.createServer({ origin: "*" });

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
            resolve();
          });
        } catch (error) {
          handleError(error);
        }
      });

      const address = server.address();
      if (!address || typeof address === "string") {
        throw new Error("Unable to determine torrent server address");
      }

      this.streamBaseUrl = `http://${TORRENT_SERVER_HOST}:${address.port}`;
      return this.streamBaseUrl;
    })().finally(() => {
      this.serverPromise = null;
    });

    return this.serverPromise;
  }
}

module.exports = { TorrentBridge, formatTorrentSnapshot };
