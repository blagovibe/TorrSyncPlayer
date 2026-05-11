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

function formatTorrentFile(file, index) {
  const extension = getFileExtension(file.name);
  const kind = getMediaKind(extension);
  if (!kind) {
    return null;
  }

  return {
    index,
    name: file.name,
    length: file.length ?? 0,
    kind,
    extension,
    progress: typeof file.progress === "number" ? file.progress : 0,
    streamUrl: typeof file.streamURL === "string" ? file.streamURL : undefined,
  };
}

function formatTorrentSnapshot(torrent, discoveredPeerCount = 0) {
  const files = torrent.files
    .map((file, index) => formatTorrentFile(file, index))
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

    return formatTorrentSnapshot(this.activeTorrent, this.discoveredPeerIds.size);
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

  async addSource(torrentSource) {
    await this.clear();

    const client = await this.getClient();
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

    if (typeof torrent.createServer === "function") {
      torrent.createServer({ origin: "*" });
    }

    this.activeTorrent = torrent;
    return formatTorrentSnapshot(torrent, this.discoveredPeerIds.size);
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
}

module.exports = { TorrentBridge, formatTorrentSnapshot };
