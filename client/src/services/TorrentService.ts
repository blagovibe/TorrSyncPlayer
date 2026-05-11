type MediaKind = "video" | "audio";

interface TorrentFile {
  index?: number;
  name: string;
  length?: number;
  progress?: number;
  streamUrl?: string;
  streamTo: (mediaElement: HTMLMediaElement) => Promise<void>;
  blob?: () => Promise<Blob>;
}

export interface TorrentMediaFile {
  index: number;
  name: string;
  length: number;
  kind: MediaKind;
  extension: string;
  file: TorrentFile;
}

interface TorrentInstance {
  files: TorrentFile[];
  progress: number;
  downloadSpeed: number;
  numPeers: number;
  on?: (
    event: "download" | "metadata" | "ready" | "error" | "wire" | "noPeers",
    callback: (...args: any[]) => void,
  ) => void;
  destroy?: (callback?: (error?: Error) => void) => void;
}

type TorrentEvents = {
  progress: (progress: number) => void;
  speed: (bytesPerSecond: number) => void;
  peerCount: (peerCount: number) => void;
  metadata: (torrent: TorrentInstance, mediaFile: TorrentMediaFile) => void;
  ready: (torrent: TorrentInstance, mediaFile: TorrentMediaFile) => void;
  error: (error: Error) => void;
};

type EventKey = keyof TorrentEvents;
type TorrentSource = string | Uint8Array;
type TorrentClient = { add: (torrentSource: TorrentSource) => TorrentInstance; destroy: () => void };
type ElectronTorrentBackend = {
  addMagnet: (magnetLink: string) => Promise<TorrentInstance>;
  addTorrentFile: (torrentFile: Uint8Array) => Promise<TorrentInstance>;
  getStats: () => Promise<TorrentInstance | null>;
  clear: () => Promise<void>;
};

type WindowWithElectronTorrent = Window & {
  torrsyncElectronTorrent?: ElectronTorrentBackend;
};

const WEBTORRENT_WEBRTC_TRACKERS = [
  "wss://tracker.btorrent.xyz",
  "wss://tracker.openwebtorrent.com",
  "wss://tracker.webtorrent.dev",
];

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

function getFileExtension(name: string): string {
  const normalized = name.trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) {
    return "";
  }
  return normalized.slice(lastDot);
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return "Unknown size";
  }

  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

export class TorrentService {
  private client: TorrentClient | null = null;
  private readonly electronBackend: ElectronTorrentBackend | null = this.getElectronBackend();
  private activeTorrent: TorrentInstance | null = null;
  private activeObjectUrl: string | null = null;
  private streamServerReady = false;
  private streamServerPromise: Promise<void> | null = null;
  private backendStatsTimer: number | null = null;
  private backendStatsInFlight = false;
  private backendCleanupPromise: Promise<void> = Promise.resolve();
  private listeners: { [K in EventKey]: Set<TorrentEvents[K]> } = {
    progress: new Set(),
    speed: new Set(),
    peerCount: new Set(),
    metadata: new Set(),
    ready: new Set(),
    error: new Set(),
  };

  on<K extends EventKey>(event: K, callback: TorrentEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  async addMagnet(magnetLink: string): Promise<TorrentInstance> {
    return this.addTorrentSource(magnetLink);
  }

  async addTorrentFile(torrentFile: Uint8Array): Promise<TorrentInstance> {
    return this.addTorrentSource(torrentFile);
  }

  isElectronBackendEnabled(): boolean {
    return this.electronBackend !== null;
  }

  getPlayableMediaFiles(torrent: TorrentInstance): TorrentMediaFile[] {
    return torrent.files
      .map((file, index) => {
        const extension = getFileExtension(file.name);
        const kind = this.getMediaKind(extension);
        if (!kind) {
          return null;
        }

        return {
          index,
          name: file.name,
          length: file.length ?? 0,
          kind,
          extension,
          file,
        } satisfies TorrentMediaFile;
      })
      .filter((file): file is TorrentMediaFile => file !== null)
      .sort((left, right) => {
        if (left.kind !== right.kind) {
          return left.kind === "video" ? -1 : 1;
        }
        if (right.length !== left.length) {
          return right.length - left.length;
        }
        return left.name.localeCompare(right.name);
      });
  }

  getPreferredMediaFile(torrent: TorrentInstance): TorrentMediaFile {
    const playableFiles = this.getPlayableMediaFiles(torrent);
    if (playableFiles.length === 0) {
      throw new Error("No supported video or audio file found in torrent");
    }

    return playableFiles[0];
  }

  private async addTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    await this.clearActiveTorrentForAdd();

    if (this.electronBackend) {
      return this.addElectronTorrentSource(torrentSource);
    }

    const client = await this.getClient();

    return new Promise<TorrentInstance>((resolve, reject) => {
      const torrent = client.add(torrentSource);
      this.activeTorrent = torrent;
      const torrentEvents = torrent.on;
      if (!torrentEvents) {
        reject(new Error("Torrent client event API is unavailable"));
        return;
      }
      let isResolved = false;
      let isRejected = false;

      const emitPeerCount = () => {
        this.emit("peerCount", torrent.numPeers ?? 0);
      };

      const settleResolve = () => {
        if (isResolved || isRejected) {
          return;
        }
        isResolved = true;
        resolve(torrent);
      };

      const settleReject = (error: Error) => {
        if (isResolved || isRejected) {
          return;
        }
        isRejected = true;
        reject(error);
      };

      torrentEvents("download", () => {
        this.emit("progress", torrent.progress);
        this.emit("speed", torrent.downloadSpeed);
        emitPeerCount();
      });

      torrentEvents("wire", (wire: { on?: (event: string, callback: () => void) => void }) => {
        emitPeerCount();
        wire?.on?.("close", emitPeerCount);
      });
      torrentEvents("noPeers", emitPeerCount);

      torrentEvents("metadata", () => {
        try {
          const videoFile = this.getPreferredMediaFile(torrent);
          this.emit("metadata", torrent, videoFile);
          this.emit("progress", torrent.progress);
          this.emit("speed", torrent.downloadSpeed);
          emitPeerCount();
          settleResolve();
        } catch (error) {
          const normalized = this.normalizeError(error);
          this.emit("error", normalized);
          settleReject(normalized);
        }
      });

      torrentEvents("ready", async () => {
        if (isRejected) {
          return;
        }
        try {
          const videoFile = this.getPreferredMediaFile(torrent);
          this.emit("ready", torrent, videoFile);
          this.emit("progress", torrent.progress);
          this.emit("speed", torrent.downloadSpeed);
          emitPeerCount();
          settleResolve();
        } catch (error) {
          const normalized = this.normalizeError(error);
          this.emit("error", normalized);
          settleReject(normalized);
        }
      });

      torrentEvents("error", (error?: Error) => {
        const normalized = this.normalizeError(error);
        this.emit("error", normalized);
        settleReject(normalized);
      });
    });
  }

  private async addElectronTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    const backend = this.electronBackend;
    if (!backend) {
      throw new Error("Electron torrent backend is unavailable");
    }

    try {
      const torrent =
        typeof torrentSource === "string"
          ? await backend.addMagnet(torrentSource)
          : await backend.addTorrentFile(torrentSource);

      this.activeTorrent = torrent;
      this.emitTorrentStats(torrent);
      this.startBackendStatsPolling();
      return torrent;
    } catch (error) {
      const normalized = this.normalizeError(error);
      this.emit("error", normalized);
      throw normalized;
    }
  }

  getVideoFile(torrent: TorrentInstance): TorrentFile {
    return this.getPreferredMediaFile(torrent).file;
  }

  async streamToMedia(file: TorrentFile, mediaElement: HTMLMediaElement): Promise<void> {
    this.revokeActiveObjectUrl();
    mediaElement.pause();
    mediaElement.removeAttribute("src");
    mediaElement.load();

    if (file.streamUrl) {
      mediaElement.src = file.streamUrl;
      mediaElement.load();
      return;
    }

    await this.ensureStreamServer();

    try {
      await file.streamTo(mediaElement);
      mediaElement.load();
      return;
    } catch (error) {
      if (!this.isMissingServerError(error) || !file.blob) {
        throw error;
      }
    }

    const blob = await file.blob();
    const objectUrl = URL.createObjectURL(blob);
    this.activeObjectUrl = objectUrl;
    mediaElement.src = objectUrl;
    mediaElement.load();
  }

  formatMediaFileLabel(mediaFile: TorrentMediaFile): string {
    return formatBytes(mediaFile.length);
  }

  clearActiveTorrent(): void {
    void this.clearActiveTorrentForAdd();
  }

  async clearActiveTorrentForAdd(): Promise<void> {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    this.stopBackendStatsPolling();
    this.revokeActiveObjectUrl();
    if (this.electronBackend) {
      this.backendCleanupPromise = this.electronBackend.clear().catch(() => undefined);
      await this.backendCleanupPromise;
      return;
    }

    if (torrent?.destroy) {
      torrent.destroy();
    }
  }

  destroy(): void {
    void this.clearActiveTorrentForAdd();
    this.client?.destroy();
    this.client = null;
    this.streamServerReady = false;
    this.streamServerPromise = null;
    this.stopBackendStatsPolling();
  }

  private async getClient(): Promise<TorrentClient> {
    if (!this.client) {
      const { default: WebTorrent } = await import("webtorrent");
      this.client = new WebTorrent({
        tracker: {
          announce: WEBTORRENT_WEBRTC_TRACKERS,
        },
      }) as TorrentClient;
    }
    return this.client;
  }

  private getElectronBackend(): ElectronTorrentBackend | null {
    if (typeof window === "undefined") {
      return null;
    }

    return (window as WindowWithElectronTorrent).torrsyncElectronTorrent ?? null;
  }

  private startBackendStatsPolling(): void {
    this.stopBackendStatsPolling();
    this.backendStatsTimer = window.setInterval(() => {
      void this.refreshBackendStats();
    }, 500);
    void this.refreshBackendStats();
  }

  private stopBackendStatsPolling(): void {
    if (this.backendStatsTimer !== null) {
      clearInterval(this.backendStatsTimer);
      this.backendStatsTimer = null;
    }
    this.backendStatsInFlight = false;
  }

  private async refreshBackendStats(): Promise<void> {
    if (this.backendStatsInFlight || !this.electronBackend || !this.activeTorrent) {
      return;
    }

    this.backendStatsInFlight = true;
    try {
      const snapshot = await this.electronBackend.getStats();
      if (!snapshot || !this.activeTorrent) {
        return;
      }

      this.mergeTorrentSnapshot(this.activeTorrent, snapshot);
      this.emitTorrentStats(this.activeTorrent);
    } catch (error) {
      this.emit("error", this.normalizeError(error));
    } finally {
      this.backendStatsInFlight = false;
    }
  }

  private mergeTorrentSnapshot(target: TorrentInstance, snapshot: TorrentInstance): void {
    target.progress = snapshot.progress ?? target.progress;
    target.downloadSpeed = snapshot.downloadSpeed ?? target.downloadSpeed;
    target.numPeers = snapshot.numPeers ?? target.numPeers;

    const filesByIndex = new Map(
      snapshot.files.map((file, index) => [typeof file.index === "number" ? file.index : index, file]),
    );
    for (const [index, file] of target.files.entries()) {
      const snapshotFile = filesByIndex.get(index);
      if (!snapshotFile) {
        continue;
      }
      file.progress = snapshotFile.progress ?? file.progress;
      file.length = snapshotFile.length ?? file.length;
      file.streamUrl = snapshotFile.streamUrl ?? file.streamUrl;
    }
  }

  private emitTorrentStats(torrent: TorrentInstance): void {
    this.emit("progress", torrent.progress);
    this.emit("speed", torrent.downloadSpeed);
    this.emit("peerCount", torrent.numPeers ?? 0);
  }

  private async ensureStreamServer(): Promise<void> {
    if (this.streamServerReady) {
      return;
    }

    if (this.streamServerPromise) {
      return this.streamServerPromise;
    }

    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) {
      return;
    }

    this.streamServerPromise = (async () => {
      await navigator.serviceWorker.register("webtorrent-sw.js");
      const readyRegistration = await navigator.serviceWorker.ready;
      const client = this.client;

      if (!client || this.streamServerReady) {
        return;
      }

      const serverCreator = (client as TorrentClient & {
        createServer?: (options: { controller: ServiceWorkerRegistration }) => unknown;
      }).createServer;

      if (typeof serverCreator !== "function") {
        return;
      }

      if (!readyRegistration.active || readyRegistration.active.state !== "activated") {
        return;
      }

      serverCreator.call(client, { controller: readyRegistration });
      this.streamServerReady = true;
    })()
      .catch(() => undefined)
      .finally(() => {
        this.streamServerPromise = null;
      });

    return this.streamServerPromise;
  }

  private revokeActiveObjectUrl(): void {
    if (!this.activeObjectUrl) {
      return;
    }

    URL.revokeObjectURL(this.activeObjectUrl);
    this.activeObjectUrl = null;
  }

  private isMissingServerError(error: unknown): boolean {
    return error instanceof Error && error.message === "No server created";
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<TorrentEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<TorrentEvents[K]>) => void)(...args);
    }
  }

  private getMediaKind(extension: string): MediaKind | null {
    if (VIDEO_EXTENSIONS.has(extension)) {
      return "video";
    }
    if (AUDIO_EXTENSIONS.has(extension)) {
      return "audio";
    }
    return null;
  }

  private normalizeError(error: unknown): Error {
    if (error instanceof Error) {
      return error;
    }
    return new Error(typeof error === "string" ? error : "Unknown torrent error");
  }
}

export default TorrentService;
