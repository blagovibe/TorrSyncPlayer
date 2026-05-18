import type { AudioTrackInfo } from "./types";
import { formatBytes } from "../utils/format";
import { createCleanup, type CleanupHandle } from "../utils/cleanup";
import { torrentLogger } from "../utils/logger";
import {
  TORRENT_CONFIG,
  STREAM_CONFIG,
  getTrackerUrls,
  isVideoExtension,
  isAudioExtension,
  getVideoPreference,
} from "../config";

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
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  on?: (event: string, callback: (...args: any[]) => void) => void;
  destroy?: (callback?: (error?: Error) => void) => void;
  select?: (start: number, end: number, priority: number) => void;
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
type TorrentClient = {
  add: (torrentSource: TorrentSource, callback?: (torrent: TorrentInstance) => void) => TorrentInstance | null;
  destroy: (callback?: () => void) => void;
  createServer?: (options: { controller: ServiceWorkerRegistration }) => unknown;
  _server?: { close?: () => void };
};

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
  createMultiplexedStreamUrl?: (params: {
    streamUrl: string;
    audioTrackIndex: number;
    startSeconds: number;
  }) => Promise<string>;
};

type WindowWithElectronTorrent = Window & {
  torrsyncElectronTorrent?: ElectronTorrentBackend;
};

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

function getFileExtension(name: string): string {
  const normalized = name.trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) return "";
  return normalized.slice(lastDot);
}

// Formats natively supported by most browsers for <video> element
const BROWSER_SUPPORTED_VIDEO_FORMATS = new Set([".mp4", ".webm", ".ogv", ".mov"]);
const BROWSER_SUPPORTED_AUDIO_FORMATS = new Set([".mp3", ".ogg", ".opus", ".wav", ".oga", ".aac", ".m4a", ".flac", ".wma"]);

export class TorrentService {
  private client: TorrentClient | null = null;
  private readonly electronBackend: ElectronTorrentBackend | null = this.getElectronBackend();
  private activeTorrent: TorrentInstance | null = null;
  private activeObjectUrl: string | null = null;
  private activeBlobUrls = new Set<string>();
  private registeredServiceWorker: ServiceWorkerRegistration | null = null;
  private activeMediaFile: TorrentMediaFile | null = null;
  private streamServerReady = false;
  private streamServerPromise: Promise<void> | null = null;
  private readonly cleanup: CleanupHandle = createCleanup();
  private backendStatsInFlight = false;
  private discoveredPeerIds = new Set<string>();
  private bufferWindowBytes: number;
  private maxBufferBytes: number;
  private currentPlaybackBytes = 0;
  private lastBufferWindow: { start: number; end: number } | null = null;
  private prioritizeTimerId: ReturnType<typeof setTimeout> | null = null;
  private addQueue: Promise<void> = Promise.resolve();
  private isDestroyed = false;

  /** Check if a file format is natively supported by browsers for <video>/<audio> */
  private isBrowserSupportedFormat(fileName: string): boolean {
    const ext = getFileExtension(fileName);
    return BROWSER_SUPPORTED_VIDEO_FORMATS.has(ext) || BROWSER_SUPPORTED_AUDIO_FORMATS.has(ext);
  }

  private readonly listeners: { [K in EventKey]: Set<TorrentEvents[K]> } = {
    progress: new Set(),
    speed: new Set(),
    peerCount: new Set(),
    metadata: new Set(),
    ready: new Set(),
    error: new Set(),
  };

  constructor() {
    this.bufferWindowBytes =
      loadBufferSetting(TORRENT_CONFIG.bufferWindowStorageKey, TORRENT_CONFIG.defaultBufferWindowMB) *
      1024 *
      1024;
    this.maxBufferBytes =
      loadBufferSetting(TORRENT_CONFIG.maxBufferStorageKey, TORRENT_CONFIG.defaultMaxBufferMB) *
      1024 *
      1024;
  }

  setBufferSettings(bufferWindowMB: number, maxBufferMB: number): void {
    const windowBytes = Math.max(1, bufferWindowMB) * 1024 * 1024;
    const maxBytes = Math.max(1, maxBufferMB) * 1024 * 1024;
    this.bufferWindowBytes = windowBytes;
    this.maxBufferBytes = maxBytes;
    saveBufferSetting(TORRENT_CONFIG.bufferWindowStorageKey, bufferWindowMB);
    saveBufferSetting(TORRENT_CONFIG.maxBufferStorageKey, maxBufferMB);
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
        if (!kind) return null;
        return { index, name: file.name, length: file.length ?? 0, kind, extension, file };
      })
      .filter((file): file is TorrentMediaFile => file !== null)
      .sort((left, right) => {
        if (left.kind !== right.kind) return left.kind === "video" ? -1 : 1;
        if (left.kind === "video" && right.kind === "video") {
          const lp = getVideoPreference(left.extension);
          const rp = getVideoPreference(right.extension);
          if (rp !== lp) return rp - lp;
        }
        if (right.length !== left.length) return right.length - left.length;
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
    if (!this.electronBackend?.probeAudioTracks || !file.streamUrl) return [];
    try {
      return await this.electronBackend.probeAudioTracks(file.streamUrl);
    } catch (error) {
      torrentLogger.warn("Audio track probe failed", error);
      return [];
    }
  }

  async createAudioTrackStreamUrl(
    file: TorrentFile,
    trackIndex: number,
    startSeconds: number,
  ): Promise<string | null> {
    if (!this.electronBackend?.createAudioTrackStreamUrl || !file.streamUrl) return null;
    try {
      return await this.electronBackend.createAudioTrackStreamUrl({
        streamUrl: file.streamUrl,
        trackIndex,
        startSeconds,
      });
    } catch (error) {
      torrentLogger.warn("Audio track stream creation failed", error);
      return null;
    }
  }

  async createMuxStreamUrl(
    file: TorrentFile,
    audioTrackIndex: number | null,
    startSeconds: number,
  ): Promise<string | null> {
    if (!this.electronBackend?.createMultiplexedStreamUrl || !file.streamUrl) return null;
    try {
      return await this.electronBackend.createMultiplexedStreamUrl({
        streamUrl: file.streamUrl,
        audioTrackIndex: audioTrackIndex ?? 0,
        startSeconds,
      });
    } catch (error) {
      torrentLogger.warn("Mux stream creation failed", error);
      return null;
    }
  }

  private async addTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    // Queue additions to prevent race conditions — each add waits for the previous.
    const prevQueue = this.addQueue;
    let releaseQueue: () => void;
    this.addQueue = new Promise<void>((resolve) => { releaseQueue = resolve; });
    await prevQueue;

    if (this.isDestroyed) {
      releaseQueue!();
      throw new Error("TorrentService has been destroyed");
    }

    try {
      await this.clearActiveTorrentForAdd();
      if (this.isDestroyed) throw new Error("TorrentService has been destroyed");

      if (this.electronBackend) {
        return this.addElectronTorrentSource(torrentSource);
      }
      return this.addWebTorrentSource(torrentSource);
    } finally {
      releaseQueue!();
    }
  }

  private async addWebTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    const client = await this.getClient();

    return new Promise<TorrentInstance>((resolve, reject) => {
      let isSettled = false;
      const settleResolve = (torrent: TorrentInstance) => {
        if (isSettled) return;
        isSettled = true;
        resolve(torrent);
      };
      const settleReject = (error: Error) => {
        if (isSettled) return;
        isSettled = true;
        reject(error);
      };

      let torrent: TorrentInstance;
      try {
        const raw = client.add(torrentSource);
        if (!raw || typeof raw !== "object") {
          settleReject(new Error("Torrent client failed to create a torrent instance"));
          return;
        }
        torrent = raw;
      } catch (error) {
        settleReject(error instanceof Error ? error : new Error(String(error)));
        return;
      }

      this.activeTorrent = torrent;

      if (!torrent.on) {
        settleReject(new Error("Torrent client event API is unavailable"));
        return;
      }

      const discoveredPeerIds = this.discoveredPeerIds;

      const emitPeerCount = () => {
        torrent.discoveredPeerCount = discoveredPeerIds.size;
        this.emit("peerCount", discoveredPeerIds.size);
      };

      // Track unique discovered peers
      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "peer", (peerId: unknown) => {
        const normalized = this.normalizePeerId(peerId);
        if (normalized && !discoveredPeerIds.has(normalized)) {
          discoveredPeerIds.add(normalized);
          emitPeerCount();
        }
      });

      // Track wire connections for peer count
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const onWire = (wire: any) => {
        emitPeerCount();
        if (wire?.on) {
          const closeHandler = () => emitPeerCount();
          wire.on("close", closeHandler);
          this.cleanup.add(() => wire.off?.("close", closeHandler));
        }
      };
      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "wire", onWire as (...args: unknown[]) => void);

      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "noPeers", emitPeerCount);

      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "download", () => {
        this.emit("progress", torrent.progress);
        this.emit("speed", torrent.downloadSpeed);
      });

      const onMetadataOrReady = (event: "metadata" | "ready") => async () => {
        if (isSettled && event === "ready") return; // metadata already resolved
        try {
          const videoFile = this.getPreferredMediaFile(torrent);
          this.emit(event, torrent, videoFile);
          this.emit("progress", torrent.progress);
          this.emit("speed", torrent.downloadSpeed);
          emitPeerCount();
          settleResolve(torrent);
        } catch (error) {
          const normalized = this.normalizeError(error);
          this.emit("error", normalized);
          settleReject(normalized);
        }
      };

      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "metadata", onMetadataOrReady("metadata"));
      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "ready", onMetadataOrReady("ready"));

      this.cleanup.on(torrent as unknown as Parameters<typeof this.cleanup.on>[0], "error", ((error?: Error) => {
        const normalized = this.normalizeError(error);
        this.emit("error", normalized);
        settleReject(normalized);
      }) as (...args: unknown[]) => void);
    });
  }

  private async addElectronTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    const backend = this.electronBackend;
    if (!backend) throw new Error("Electron torrent backend is unavailable");

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
    this.activeMediaFile = this.activeTorrent
      ? this.getPlayableMediaFiles(this.activeTorrent).find((mf) => mf.file === file) ?? null
      : null;
    mediaElement.pause();
    mediaElement.removeAttribute("src");
    mediaElement.load();

    if (file.streamUrl) {
      // For formats not natively supported by browsers (e.g. MKV), use mux conversion
      const isNativeSupported = this.isBrowserSupportedFormat(file.name);
      if (!isNativeSupported && this.electronBackend?.createMultiplexedStreamUrl) {
        torrentLogger.info(`Format not browser-supported, using mux conversion for: ${file.name}`);
        const muxUrl = await this.createMuxStreamUrl(
          file,
          null, // use default audio track
          0,
        );
        if (muxUrl) {
          await this.streamFromUrl(muxUrl, mediaElement);
          return;
        }
        torrentLogger.warn("Mux conversion failed, falling back to direct stream");
      }
      await this.streamFromUrl(file.streamUrl, mediaElement);
      return;
    }

    try {
      await this.ensureStreamServer();
    } catch (error) {
      torrentLogger.warn("Stream server setup failed, falling back to blob", error);
    }

    try {
      await file.streamTo(mediaElement);
      mediaElement.load();
      return;
    } catch (error) {
      if (!this.isMissingServerError(error) || !file.blob) {
        throw error;
      }
    }

    // Fallback: download as blob and create object URL
    const blob = await file.blob();
    const objectUrl = URL.createObjectURL(blob);
    this.activeObjectUrl = objectUrl;
    this.activeBlobUrls.add(objectUrl);
    mediaElement.src = objectUrl;
    mediaElement.load();
  }

  private streamFromUrl(streamUrl: string, mediaElement: HTMLMediaElement): Promise<void> {
    return new Promise((resolve, reject) => {
      let settled = false;
      const settle = (fn: () => void) => {
        if (settled) return;
        settled = true;
        fn();
      };

      const cleanup = () => {
        mediaElement.removeEventListener("error", onError);
        mediaElement.removeEventListener("canplay", onCanPlay);
        mediaElement.removeEventListener("loadeddata", onLoadedData);
      };

      const onError = () => {
        cleanup();
        const error = mediaElement.error;
        const errorMsg = error ? `[${error.code}] ${error.message}` : "unknown error";
        settle(() => reject(new Error(`Failed to load stream from URL: ${streamUrl} (${errorMsg})`)));
      };

      const onCanPlay = () => {
        cleanup();
        settle(resolve);
      };

      const onLoadedData = () => {
        // loadeddata fires when first frame is available — good enough for streaming
        cleanup();
        settle(resolve);
      };

      // Timeout: if neither error nor canplay fires within 60s, reject
      this.cleanup.setTimeout(() => {
        cleanup();
        settle(() => reject(new Error(`Stream load timed out: ${streamUrl}`)));
      }, 60_000);

      mediaElement.addEventListener("error", onError);
      mediaElement.addEventListener("canplay", onCanPlay);
      mediaElement.addEventListener("loadeddata", onLoadedData);
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
    this.activeMediaFile = null;
    this.lastBufferWindow = null;
    this.stopPrioritizeLoop();
    this.stopBackendStatsPolling();
    this.revokeActiveObjectUrl();
    this.revokeAllBlobUrls();
    this.discoveredPeerIds.clear();

    if (this.electronBackend) {
      try {
        await this.electronBackend.clear();
      } catch {
        // Ignore cleanup errors
      }
      return;
    }

    const destroyTorrent = torrent?.destroy;
    if (typeof destroyTorrent === "function") {
      await new Promise<void>((resolve) => {
        let resolved = false;
        const settle = () => {
          if (resolved) return;
          resolved = true;
          resolve();
        };
        const timeoutId = globalThis.setTimeout(settle, STREAM_CONFIG.torrentDestroyTimeoutMs);
        try {
          if (destroyTorrent.length > 0) {
            destroyTorrent(() => {
              globalThis.clearTimeout(timeoutId);
              settle();
            });
            return;
          }
          destroyTorrent();
        } catch {
          // Ignore cleanup errors
        }
        globalThis.clearTimeout(timeoutId);
        settle();
      });
    }
  }

  /**
   * Full destruction of the service. After calling this, the service cannot be reused.
   */
  async destroy(): Promise<void> {
    this.isDestroyed = true;
    this.cleanup.abort();
    await this.clearActiveTorrentForAdd();

    if (this.client && typeof this.client.destroy === "function") {
      await new Promise<void>((resolve) => {
        try {
          this.client!.destroy(() => resolve());
        } catch {
          resolve();
        }
      });
    }
    this.client = null;
    this.streamServerReady = false;
    this.streamServerPromise = null;
    this.revokeAllBlobUrls();
    this.revokeActiveObjectUrl();

    if (this.registeredServiceWorker) {
      void this.registeredServiceWorker.unregister().catch(() => undefined);
      this.registeredServiceWorker = null;
    }

    // Clear all listeners to prevent stale callbacks
    for (const key of Object.keys(this.listeners) as EventKey[]) {
      this.listeners[key].clear();
    }
  }

  updatePlaybackPosition(currentTimeSeconds: number, fileLengthBytes: number, fileDurationSeconds: number): void {
    if (fileDurationSeconds <= 0 || fileLengthBytes <= 0) return;
    const bytesPerSecond = fileLengthBytes / fileDurationSeconds;
    const newPlaybackBytes = Math.floor(currentTimeSeconds * bytesPerSecond);
    const jumpThreshold = this.bufferWindowBytes * 0.5;
    if (Math.abs(newPlaybackBytes - this.currentPlaybackBytes) > jumpThreshold) {
      this.currentPlaybackBytes = newPlaybackBytes;
      this.prioritizeNow();
    } else {
      this.currentPlaybackBytes = newPlaybackBytes;
      this.schedulePrioritize();
    }
  }

  getBufferWindow(): { startMB: number; endMB: number; maxSizeMB: number } {
    return {
      startMB: Math.round(Math.max(0, this.currentPlaybackBytes - this.bufferWindowBytes) / 1024 / 1024),
      endMB: Math.round((this.currentPlaybackBytes + this.bufferWindowBytes) / 1024 / 1024),
      maxSizeMB: Math.round(this.maxBufferBytes / 1024 / 1024),
    };
  }

  private schedulePrioritize(): void {
    if (this.prioritizeTimerId !== null) return; // already scheduled
    this.prioritizeTimerId = this.cleanup.setTimeout(() => {
      this.prioritizeTimerId = null;
      this.applyBufferPriority();
    }, TORRENT_CONFIG.prioritizeIntervalMs);
  }

  prioritizeNow(): void {
    this.stopPrioritizeLoop();
    this.applyBufferPriority();
  }

  private stopPrioritizeLoop(): void {
    if (this.prioritizeTimerId !== null) {
      clearTimeout(this.prioritizeTimerId);
      this.prioritizeTimerId = null;
    }
  }

  private applyBufferPriority(): void {
    const torrent = this.activeTorrent;
    if (!torrent?.select || !torrent?.deselect) return;

    const mediaFile = this.activeMediaFile;
    const file = mediaFile?.file ?? torrent.files.find((f) => f.length && f.length > 0);
    if (!file?.length) return;

    const fileStart = 0;
    const fileEnd = file.length - 1;
    const bufferStart = Math.max(fileStart, this.currentPlaybackBytes - this.bufferWindowBytes);
    const bufferEnd = Math.min(fileEnd, this.currentPlaybackBytes + this.bufferWindowBytes);

    const prev = this.lastBufferWindow;
    if (prev) {
      if (bufferStart > prev.start) {
        torrent.deselect(prev.start, Math.min(bufferStart - 1, prev.end), 0);
      }
      if (bufferEnd < prev.end) {
        torrent.deselect(Math.max(bufferEnd + 1, prev.start), prev.end, 0);
      }
      if (bufferStart < prev.start) {
        torrent.select(bufferStart, prev.start - 1, 1);
      }
      if (bufferEnd > prev.end) {
        torrent.select(prev.end + 1, bufferEnd, 1);
      }
    } else {
      torrent.deselect(fileStart, fileEnd, 0);
      torrent.select(bufferStart, bufferEnd, 1);
    }

    this.lastBufferWindow = { start: bufferStart, end: bufferEnd };
  }

  private async getClient(): Promise<TorrentClient> {
    if (!this.client) {
      const { default: WebTorrent } = await import("webtorrent");
      this.client = new WebTorrent({
        maxConns: TORRENT_CONFIG.maxConnections,
        tracker: { announce: getTrackerUrls() },
      }) as TorrentClient;
    }
    return this.client;
  }

  private cachedElectronBackend: ElectronTorrentBackend | null | undefined = undefined;

  private getElectronBackend(): ElectronTorrentBackend | null {
    if (this.cachedElectronBackend !== undefined) return this.cachedElectronBackend;
    if (typeof window === "undefined") {
      this.cachedElectronBackend = null;
      return null;
    }
    this.cachedElectronBackend = (window as WindowWithElectronTorrent).torrsyncElectronTorrent ?? null;
    return this.cachedElectronBackend;
  }

  private startBackendStatsPolling(): void {
    this.stopBackendStatsPolling();
    this.cleanup.setInterval(() => {
      void this.refreshBackendStats();
    }, 500);
    void this.refreshBackendStats();
  }

  private stopBackendStatsPolling(): void {
    this.backendStatsInFlight = false;
  }

  private async refreshBackendStats(): Promise<void> {
    if (this.backendStatsInFlight || !this.electronBackend || !this.activeTorrent) return;
    this.backendStatsInFlight = true;
    try {
      const snapshot = await this.electronBackend.getStats();
      if (!snapshot || !this.activeTorrent) return;
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

    const filesByIndex = new Map<number, TorrentFile>();
    for (let i = 0; i < snapshot.files.length; i++) {
      const file = snapshot.files[i];
      const idx = typeof file.index === "number" ? file.index : i;
      filesByIndex.set(idx, file);
    }
    for (const [index, file] of target.files.entries()) {
      const snapshotFile = filesByIndex.get(index);
      if (!snapshotFile) continue;
      file.progress = snapshotFile.progress ?? file.progress;
      file.length = snapshotFile.length ?? file.length;
      file.streamUrl = snapshotFile.streamUrl ?? file.streamUrl;
    }
    for (const [index, snapshotFile] of filesByIndex.entries()) {
      if (index >= target.files.length) {
        target.files[index] = { ...snapshotFile };
      }
    }
  }

  private emitTorrentStats(torrent: TorrentInstance): void {
    this.emit("progress", torrent.progress);
    this.emit("speed", torrent.downloadSpeed);
    this.emit("peerCount", torrent.discoveredPeerCount ?? this.discoveredPeerIds.size);
  }

  private async ensureStreamServer(): Promise<void> {
    if (this.streamServerReady) return;
    if (this.streamServerPromise) return this.streamServerPromise;
    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;

    this.streamServerPromise = (async () => {
      try {
        await navigator.serviceWorker.register("webtorrent-sw.js");
        this.registeredServiceWorker = await navigator.serviceWorker.ready;
        const client = this.client;
        if (!client || this.streamServerReady) return;

        const serverCreator = (client as TorrentClient & {
          createServer?: (options: { controller: ServiceWorkerRegistration }) => unknown;
        }).createServer;

        if (typeof serverCreator !== "function") return;
        if (!this.registeredServiceWorker?.active || this.registeredServiceWorker.active.state !== "activated") return;

        serverCreator.call(client, { controller: this.registeredServiceWorker });
        this.streamServerReady = true;
      } catch (error) {
        torrentLogger.warn("Stream server setup failed", error);
        // Reset promise so next attempt can retry
        this.streamServerPromise = null;
      }
    })();

    return this.streamServerPromise;
  }

  private revokeActiveObjectUrl(): void {
    if (!this.activeObjectUrl) return;
    URL.revokeObjectURL(this.activeObjectUrl);
    this.activeObjectUrl = null;
  }

  private revokeAllBlobUrls(): void {
    for (const url of this.activeBlobUrls) {
      URL.revokeObjectURL(url);
    }
    this.activeBlobUrls.clear();
  }

  private isMissingServerError(error: unknown): boolean {
    return error instanceof Error && error.message === "No server created";
  }

  private normalizePeerId(peerId: unknown): string | null {
    if (peerId == null) return null;
    const normalized = String(peerId).trim();
    return normalized.length > 0 ? normalized : null;
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<TorrentEvents[K]>): void {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<TorrentEvents[K]>) => void)(...args);
    }
  }

  private getMediaKind(extension: string): MediaKind | null {
    if (isVideoExtension(extension)) return "video";
    if (isAudioExtension(extension)) return "audio";
    return null;
  }

  private normalizeError(error: unknown): Error {
    if (error instanceof Error) return error;
    if (error == null) return new Error("Unknown torrent error");
    return new Error(typeof error === "string" ? error : "Unknown torrent error");
  }
}

export default TorrentService;
