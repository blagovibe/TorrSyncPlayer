import type { AudioTrackInfo } from "./types";
import { formatBytes } from "../utils/format";

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
  discoveredPeerCount?: number;
  on?: (
    event: "download" | "metadata" | "ready" | "error" | "wire" | "noPeers" | "peer",
    callback: (...args: any[]) => void,
  ) => void;
  destroy?: (callback?: (error?: Error) => void) => void;
  // WebTorrent-specific: select byte ranges for priority downloading.
  select?: (start: number, end: number, priority: number) => void;
  // WebTorrent-specific: deselect all pieces.
  deselect?: (start: number, end: number, priority: number) => void;
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
  probeAudioTracks?: (streamUrl: string) => Promise<AudioTrackInfo[]>;
  createAudioTrackStreamUrl?: (params: {
    streamUrl: string;
    trackIndex: number;
    startSeconds: number;
  }) => Promise<string>;
};

type WindowWithElectronTorrent = Window & {
  torrsyncElectronTorrent?: ElectronTorrentBackend;
};

const WEBTORRENT_WEBRTC_TRACKERS = [
  "wss://tracker.btorrent.xyz",
  "wss://tracker.openwebtorrent.com",
  "wss://tracker.webtorrent.dev",
];
const MAX_TORRENT_CONNECTIONS = 200;

// Default buffer window: prioritize downloading ±50MB around the current playback position.
const DEFAULT_BUFFER_WINDOW_MB = 50;
// Default maximum total buffer size: 500MB.
const DEFAULT_MAX_BUFFER_MB = 500;
// How often to re-prioritize pieces based on current time (ms).
const PRIORITIZE_INTERVAL_MS = 2000;

const BUFFER_WINDOW_STORAGE_KEY = "torrsyncplayer.bufferWindowMB";
const MAX_BUFFER_STORAGE_KEY = "torrsyncplayer.maxBufferMB";

function loadBufferSetting(storageKey: string, defaultValue: number): number {
  if (typeof window === "undefined") return defaultValue;
  try {
    const stored = window.localStorage.getItem(storageKey);
    if (stored !== null) {
      const parsed = Number(stored);
      if (Number.isFinite(parsed) && parsed > 0) return parsed;
    }
  } catch {
    // localStorage unavailable
  }
  return defaultValue;
}

function saveBufferSetting(storageKey: string, value: number): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(storageKey, String(value));
  } catch {
    // localStorage unavailable
  }
}

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

const PREFERRED_VIDEO_EXTENSIONS = new Map<string, number>([
  [".mp4", 4],
  [".m4v", 4],
  [".webm", 3],
  [".mov", 2],
  [".ogv", 2],
  [".ts", 1],
  [".mkv", 0],
  [".avi", 0],
]);

function getFileExtension(name: string): string {
  const normalized = name.trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) {
    return "";
  }
  return normalized.slice(lastDot);
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
  private discoveredPeerIds = new Set<string>();
  // Configurable buffer settings (loaded from localStorage).
  private bufferWindowBytes: number = loadBufferSetting(BUFFER_WINDOW_STORAGE_KEY, DEFAULT_BUFFER_WINDOW_MB) * 1024 * 1024;
  private maxBufferBytes: number = loadBufferSetting(MAX_BUFFER_STORAGE_KEY, DEFAULT_MAX_BUFFER_MB) * 1024 * 1024;
  // Buffer management state.
  private prioritizeTimer: number | null = null;
  private currentPlaybackBytes = 0;
  private readonly listeners: { [K in EventKey]: Set<TorrentEvents[K]> } = {
    progress: new Set(),
    speed: new Set(),
    peerCount: new Set(),
    metadata: new Set(),
    ready: new Set(),
    error: new Set(),
  };

  /** Update buffer window and max buffer sizes (in MB). */
  setBufferSettings(bufferWindowMB: number, maxBufferMB: number): void {
    const windowBytes = Math.max(1, bufferWindowMB) * 1024 * 1024;
    const maxBytes = Math.max(1, maxBufferMB) * 1024 * 1024;
    this.bufferWindowBytes = windowBytes;
    this.maxBufferBytes = maxBytes;
    saveBufferSetting(BUFFER_WINDOW_STORAGE_KEY, bufferWindowMB);
    saveBufferSetting(MAX_BUFFER_STORAGE_KEY, maxBufferMB);
  }

  getBufferSettings(): { bufferWindowMB: number; maxBufferMB: number } {
    return {
      bufferWindowMB: Math.round(this.bufferWindowBytes / 1024 / 1024),
      maxBufferMB: Math.round(this.maxBufferBytes / 1024 / 1024),
    };
  }

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
        if (left.kind === "video" && right.kind === "video") {
          const leftPriority = this.getVideoCompatibilityPriority(left.extension);
          const rightPriority = this.getVideoCompatibilityPriority(right.extension);
          if (rightPriority !== leftPriority) {
            return rightPriority - leftPriority;
          }
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

  async probeAudioTracks(file: TorrentFile): Promise<AudioTrackInfo[]> {
    if (!this.electronBackend?.probeAudioTracks || !file.streamUrl) {
      return [];
    }

    try {
      return await this.electronBackend.probeAudioTracks(file.streamUrl);
    } catch (error) {
      console.warn("Audio track probe failed:", error);
      return [];
    }
  }

  async createAudioTrackStreamUrl(
    file: TorrentFile,
    trackIndex: number,
    startSeconds: number,
  ): Promise<string | null> {
    if (!this.electronBackend?.createAudioTrackStreamUrl || !file.streamUrl) {
      return null;
    }

    try {
      return await this.electronBackend.createAudioTrackStreamUrl({
        streamUrl: file.streamUrl,
        trackIndex,
        startSeconds,
      });
    } catch (error) {
      console.warn("Audio track stream creation failed:", error);
      return null;
    }
  }

  private async addTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    await this.clearActiveTorrentForAdd();

    if (this.electronBackend) {
      return this.addElectronTorrentSource(torrentSource);
    }

    const client = await this.getClient();

    return new Promise<TorrentInstance>((resolve, reject) => {
      const torrent = client.add(torrentSource);
      if (!torrent || typeof torrent !== "object") {
        reject(new Error("Torrent client failed to create a torrent instance"));
        return;
      }
      this.activeTorrent = torrent;
      const torrentEvents = torrent.on;
      if (!torrentEvents) {
        reject(new Error("Torrent client event API is unavailable"));
        return;
      }
      let isResolved = false;
      let isRejected = false;

      const emitPeerCount = () => {
        torrent.discoveredPeerCount = this.discoveredPeerIds.size;
        this.emit("peerCount", torrent.discoveredPeerCount);
      };

      torrentEvents("peer", (peerId: unknown) => {
        if (this.recordDiscoveredPeer(peerId)) {
          emitPeerCount();
        }
      });

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
          if (!isRejected) {
            settleResolve();
          }
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
          if (!isResolved) {
            settleResolve();
          }
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
      await this.streamFromUrl(file.streamUrl, mediaElement);
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

  private streamFromUrl(streamUrl: string, mediaElement: HTMLMediaElement): Promise<void> {
    return new Promise((resolve, reject) => {
      const onError = () => {
        cleanup();
        reject(new Error(`Failed to load stream from URL: ${streamUrl}`));
      };
      const onCanPlay = () => {
        cleanup();
        resolve();
      };
      const cleanup = () => {
        mediaElement.removeEventListener("error", onError);
        mediaElement.removeEventListener("canplay", onCanPlay);
      };
      mediaElement.addEventListener("error", onError, { once: true });
      mediaElement.addEventListener("canplay", onCanPlay, { once: true });
      mediaElement.src = streamUrl;
      mediaElement.load();
    });
  }

  formatMediaFileLabel(mediaFile: TorrentMediaFile): string {
    return formatBytes(mediaFile.length);
  }

  async clearActiveTorrent(): Promise<void> {
    return this.clearActiveTorrentForAdd();
  }

  async clearActiveTorrentForAdd(): Promise<void> {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    this.stopBackendStatsPolling();
    this.revokeActiveObjectUrl();
    this.discoveredPeerIds.clear();
    if (this.electronBackend) {
      this.backendCleanupPromise = this.electronBackend.clear().catch(() => undefined);
      await this.backendCleanupPromise;
      return;
    }

    const destroyTorrent = torrent?.destroy;
    if (typeof destroyTorrent === "function") {
      await new Promise<void>((resolve) => {
        try {
          if (destroyTorrent.length > 0) {
            destroyTorrent(() => resolve());
            return;
          }

          destroyTorrent();
        } catch {
          // Ignore cleanup errors and move on to the next request.
        }
        resolve();
      });
    }
  }

  destroy(): Promise<void> {
    return this.clearActiveTorrentForAdd().then(() => {
      this.client?.destroy();
      this.client = null;
      this.streamServerReady = false;
      this.streamServerPromise = null;
      this.stopBackendStatsPolling();
      this.stopPrioritizeLoop();
    });
  }

  /**
   * Update the current playback position so the buffer window can follow.
   * Call this from the video player's timeupdate handler.
   */
  updatePlaybackPosition(currentTimeSeconds: number, fileLengthBytes: number, fileDurationSeconds: number): void {
    if (fileDurationSeconds <= 0 || fileLengthBytes <= 0) {
      return;
    }
    const bytesPerSecond = fileLengthBytes / fileDurationSeconds;
    this.currentPlaybackBytes = Math.floor(currentTimeSeconds * bytesPerSecond);
    this.schedulePrioritize();
  }

  /**
   * Get the current buffer window for display purposes.
   */
  getBufferWindow(): { start: number; end: number; maxSize: number } {
    return {
      start: Math.max(0, this.currentPlaybackBytes - this.bufferWindowBytes),
      end: this.currentPlaybackBytes + this.bufferWindowBytes,
      maxSize: this.maxBufferBytes,
    };
  }

  private schedulePrioritize(): void {
    if (this.prioritizeTimer !== null) return;
    this.prioritizeTimer = window.setTimeout(() => {
      this.prioritizeTimer = null;
      this.applyBufferPriority();
    }, PRIORITIZE_INTERVAL_MS);
  }

  private stopPrioritizeLoop(): void {
    if (this.prioritizeTimer !== null) {
      window.clearTimeout(this.prioritizeTimer);
      this.prioritizeTimer = null;
    }
  }

  private applyBufferPriority(): void {
    const torrent = this.activeTorrent;
    if (!torrent?.select || !torrent?.deselect) return;

    const file = torrent.files.find((f) => f.length && f.length > 0);
    if (!file?.length) return;

    const fileStart = 0;
    const fileEnd = file.length - 1;
    const bufferStart = Math.max(fileStart, this.currentPlaybackBytes - this.bufferWindowBytes);
    const bufferEnd = Math.min(fileEnd, this.currentPlaybackBytes + this.bufferWindowBytes);

    // Incremental update: prioritize new window first, then deselect outside.
    // This avoids a momentary gap where nothing is prioritized.
    torrent.select(bufferStart, bufferEnd, 1);
    if (bufferStart > fileStart) {
      torrent.deselect(fileStart, bufferStart - 1, 0);
    }
    if (bufferEnd < fileEnd) {
      torrent.deselect(bufferEnd + 1, fileEnd, 0);
    }
  }

  private async getClient(): Promise<TorrentClient> {
    if (!this.client) {
      const { default: WebTorrent } = await import("webtorrent");
      this.client = new WebTorrent({
        maxConns: MAX_TORRENT_CONNECTIONS,
        tracker: {
          announce: WEBTORRENT_WEBRTC_TRACKERS,
        },
      }) as TorrentClient;
    }
    return this.client;
  }

  private cachedElectronBackend: ElectronTorrentBackend | null | undefined = undefined;

  private getElectronBackend(): ElectronTorrentBackend | null {
    if (this.cachedElectronBackend !== undefined) {
      return this.cachedElectronBackend;
    }

    if (typeof window === "undefined") {
      this.cachedElectronBackend = null;
      return null;
    }

    this.cachedElectronBackend = (window as WindowWithElectronTorrent).torrsyncElectronTorrent ?? null;
    return this.cachedElectronBackend;
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
    target.discoveredPeerCount = snapshot.discoveredPeerCount ?? target.discoveredPeerCount;

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
    this.emit("peerCount", torrent.discoveredPeerCount ?? this.discoveredPeerIds.size);
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

  private getVideoCompatibilityPriority(extension: string): number {
    return PREFERRED_VIDEO_EXTENSIONS.get(extension) ?? 0;
  }

  private isMissingServerError(error: unknown): boolean {
    return error instanceof Error && error.message === "No server created";
  }

  private recordDiscoveredPeer(peerId: unknown): boolean {
    const normalizedPeerId = this.normalizePeerId(peerId);
    if (!normalizedPeerId) {
      return false;
    }

    const previousSize = this.discoveredPeerIds.size;
    this.discoveredPeerIds.add(normalizedPeerId);
    return this.discoveredPeerIds.size !== previousSize;
  }

  private normalizePeerId(peerId: unknown): string | null {
    if (peerId == null) {
      return null;
    }

    const normalized = String(peerId).trim();
    return normalized.length > 0 ? normalized : null;
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
