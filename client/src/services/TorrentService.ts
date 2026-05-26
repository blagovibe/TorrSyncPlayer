import type { AudioTrackInfo, SubtitleTrackInfo } from "./types";
import { formatBytes } from "../utils/format";
import { type ElectronTorrentBackendAdapter, getElectronBackendAdapter, type TorrentInstance, type TorrentFile } from "./torrent-backend";
export type { TorrentInstance, TorrentFile };
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

export interface TorrentMediaFile {
  index: number;
  name: string;
  length: number;
  kind: MediaKind;
  extension: string;
  file: TorrentFile;
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
  add: (torrentSource: TorrentSource, opts?: Record<string, unknown>, callback?: (torrent: TorrentInstance) => void) => TorrentInstance | null;
  destroy: (callback?: () => void) => void;
  createServer?: (options: { controller: ServiceWorkerRegistration }) => unknown;
  _server?: { close?: () => void };
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
const BROWSER_SUPPORTED_VIDEO_FORMATS = new Set([".mp4", ".webm", ".ogv", ".mov", ".m4v", ".ts"]);
const BROWSER_SUPPORTED_AUDIO_FORMATS = new Set([".mp3", ".ogg", ".opus", ".wav", ".oga", ".aac", ".m4a", ".flac", ".wma"]);

type TorrentEmitter = {
  on: (event: string, callback: (...args: unknown[]) => void) => void;
  off?: (event: string, callback: (...args: unknown[]) => void) => void;
  removeListener?: (event: string, callback: (...args: unknown[]) => void) => void;
};

type BackendType = "browser" | "electron";

export class TorrentService {
  private client: TorrentClient | null = null;
  private backendType: BackendType | null = null;

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
  private streamAbortController: AbortController | null = null;
  private bufferWindowBytes: number;
  private maxBufferBytes: number;
  private currentPlaybackBytes = 0;
  private lastBufferWindow: { start: number; end: number } | null = null;
  private prioritizeTimerId: ReturnType<typeof setTimeout> | null = null;
  private lastProgress = -1;
  private lastSpeed = -1;
  private lastPeerCount = -1;
  private statsIntervalId: ReturnType<typeof setInterval> | null = null;
  private addQueue: Promise<void> = Promise.resolve();
  private isDestroyed = false;
  private addAbortController: AbortController | null = null;
  private loadGeneration = 0;

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

  /**
   * Set buffer window and maximum buffer size.
   * @param bufferWindowMB - The buffer window size in megabytes.
   * @param maxBufferMB - The maximum total buffer size in megabytes.
   */
  setBufferSettings(bufferWindowMB: number, maxBufferMB: number): void {
    const windowBytes = Math.max(1, bufferWindowMB) * 1024 * 1024;
    const maxBytes = Math.max(1, maxBufferMB) * 1024 * 1024;
    this.bufferWindowBytes = windowBytes;
    this.maxBufferBytes = maxBytes;
    saveBufferSetting(TORRENT_CONFIG.bufferWindowStorageKey, bufferWindowMB);
    saveBufferSetting(TORRENT_CONFIG.maxBufferStorageKey, maxBufferMB);

    const adapter = this.getElectronBackend();
    if (adapter) {
      adapter.setMaxBufferMB(maxBufferMB);
    }
  }

  /** Get the current buffer settings in megabytes. */
  getBufferSettings(): { bufferWindowMB: number; maxBufferMB: number } {
    return {
      bufferWindowMB: Math.round(this.bufferWindowBytes / 1024 / 1024),
      maxBufferMB: Math.round(this.maxBufferBytes / 1024 / 1024),
    };
  }

  /**
   * Register an event listener.
   * @returns An unsubscribe function.
   */
  on<K extends EventKey>(event: K, callback: TorrentEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  /** Add a torrent from a magnet link and return the torrent instance. */
  async addMagnet(magnetLink: string): Promise<TorrentInstance> {
    return this.addTorrentSource(magnetLink);
  }

  /** Add a torrent from raw file bytes and return the torrent instance. */
  async addTorrentFile(torrentFile: Uint8Array): Promise<TorrentInstance> {
    return this.addTorrentSource(torrentFile);
  }

  /** Returns true if the Electron torrent backend is available. */
  isElectronBackendEnabled(): boolean {
    return this.getElectronBackend() !== null;
  }

  /** Get all playable media files from a torrent, sorted by preference (video first, then by size). */
  getPlayableMediaFiles(torrent: TorrentInstance): TorrentMediaFile[] {
    if (torrent.files.length > 10_000) {
      throw new Error(`Torrent contains too many files (${torrent.files.length}). Maximum allowed is 10,000.`);
    }
    for (const f of torrent.files) {
      if (typeof f.name === "string" && f.name.length > 512) {
        throw new Error(`Torrent file name exceeds maximum length: ${f.name.slice(0, 64)}…`);
      }
    }
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

  /** Get the preferred (largest, highest-priority) media file from a torrent. */
  getPreferredMediaFile(torrent: TorrentInstance): TorrentMediaFile {
    const playableFiles = this.getPlayableMediaFiles(torrent);
    if (playableFiles.length === 0) {
      // Provide more detailed error information
      const allFiles = torrent.files.map(f => f.name).join(', ');
      throw new Error(`No supported video or audio file found in torrent. Available files: ${allFiles}`);
    }
    return playableFiles[0];
  }

  /** Probe a media file for available audio tracks via the Electron backend. */
  async probeAudioTracks(file: TorrentFile): Promise<AudioTrackInfo[]> {
    const backend = this.getElectronBackend();
    if (!backend?.probeAudioTracks || !file.streamUrl) return [];
    try {
      return await backend.probeAudioTracks(file.streamUrl);
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
    const backend = this.getElectronBackend();
    if (!backend?.createAudioTrackStreamUrl || !file.streamUrl) return null;
    try {
      return await backend.createAudioTrackStreamUrl({
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
    const backend = this.getElectronBackend();
    if (!backend?.createMultiplexedStreamUrl || !file.streamUrl) return null;
    try {
      return await backend.createMultiplexedStreamUrl({
        streamUrl: file.streamUrl,
        audioTrackIndex: audioTrackIndex ?? 0,
        startSeconds,
      });
    } catch (error) {
      torrentLogger.warn("Mux stream creation failed", error);
      return null;
    }
  }

  /** Probe a media file for available subtitle tracks via the Electron backend. */
  async probeSubtitles(file: TorrentFile): Promise<SubtitleTrackInfo[]> {
    const backend = this.getElectronBackend();
    if (!backend?.probeSubtitles || !file.streamUrl) return [];
    try {
      return await backend.probeSubtitles(file.streamUrl);
    } catch (error) {
      torrentLogger.warn("Subtitle probe failed", error);
      return [];
    }
  }

  async createSubtitleStreamUrl(
    file: TorrentFile,
    trackIndex: number,
    startSeconds: number,
  ): Promise<string | null> {
    const backend = this.getElectronBackend();
    if (!backend?.createSubtitleStreamUrl || !file.streamUrl) return null;
    try {
      return await backend.createSubtitleStreamUrl({
        streamUrl: file.streamUrl,
        trackIndex,
        startSeconds,
      });
    } catch (error) {
      torrentLogger.warn("Subtitle stream creation failed", error);
      return null;
    }
  }

  private async addTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    const prevQueue = this.addQueue;
    let releaseQueue: () => void;
    this.addQueue = new Promise<void>((resolve) => { releaseQueue = resolve; });
    await prevQueue.catch(() => {});

    if (this.isDestroyed) {
      releaseQueue!();
      throw new Error("TorrentService has been destroyed");
    }

    const previousAbortController = this.addAbortController;
    previousAbortController?.abort();
    const abortController = new AbortController();
    this.addAbortController = abortController;

    const generation = ++this.loadGeneration;

    try {
      await this.clearActiveTorrentForAdd();
      if (this.isDestroyed || abortController.signal.aborted) throw new Error("TorrentService has been destroyed");

      if (this.getElectronBackend()) {
        if (this.backendType === "browser") {
          throw new Error("Cannot mix Electron and browser WebTorrent backends in the same service instance");
        }
        this.backendType = "electron";
        return this.addElectronTorrentSource(torrentSource, abortController.signal, generation);
      }
      if (this.backendType === "electron") {
        throw new Error("Cannot mix browser and Electron WebTorrent backends in the same service instance");
      }
      this.backendType = "browser";
      return this.addWebTorrentSource(torrentSource, abortController.signal, generation);
    } finally {
      if (this.loadGeneration === generation) {
        releaseQueue!();
      }
    }
  }

  private async addWebTorrentSource(torrentSource: TorrentSource, abortSignal?: AbortSignal, expectedGeneration?: number): Promise<TorrentInstance> {
    const client = await this.getClient();

    if (abortSignal?.aborted) throw new Error("Torrent load was cancelled");
    if (expectedGeneration !== undefined && this.loadGeneration !== expectedGeneration) {
      throw new Error("Torrent load superseded by a newer request");
    }

    return new Promise<TorrentInstance>((resolve, reject) => {
      let isSettled = false;
      const isStale = () => expectedGeneration !== undefined && this.loadGeneration !== expectedGeneration;
      const settleResolve = (torrent: TorrentInstance) => {
        if (isSettled) return;
        if (isStale()) {
          settleReject(new Error("Torrent load superseded by a newer request"));
          return;
        }
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

      if (abortSignal?.aborted || isStale()) {
        settleReject(new Error("Torrent load was cancelled"));
        return;
      }

    this.activeTorrent = torrent;

      if (!torrent.on) {
        settleReject(new Error("Torrent client event API is unavailable"));
        return;
      }

      const discoveredPeerIds = this.discoveredPeerIds;

      const emitDiscoveredPeerCount = () => {
        torrent.discoveredPeerCount = discoveredPeerIds.size;
        this.emit("peerCount", discoveredPeerIds.size);
      };

      // Track unique discovered peers
      const onPeer = (peerId: unknown) => {
        const normalized = this.normalizePeerId(peerId);
        if (normalized && !discoveredPeerIds.has(normalized)) {
          discoveredPeerIds.add(normalized);
          emitDiscoveredPeerCount();
        }
      };
      this.cleanup.on(torrent as unknown as TorrentEmitter, "peer", onPeer as (...args: unknown[]) => void);

      // Track wire connections separately — wire count is informational
      const onWire = (wire: { on?: (event: string, handler: () => void) => void; off?: (event: string, handler: () => void) => void }) => {
        if (wire?.on && wire?.off) {
          const closeHandler = () => {};
          wire.on("close", closeHandler);
          this.cleanup.add(() => wire.off!("close", closeHandler));
        }
      };
      this.cleanup.on(torrent as unknown as TorrentEmitter, "wire", onWire as (...args: unknown[]) => void);

      this.cleanup.on(torrent as unknown as TorrentEmitter, "noPeers", emitDiscoveredPeerCount);

      this.cleanup.on(torrent as unknown as TorrentEmitter, "download", () => {
        const prog = Math.round(torrent.progress * 100);
        const spd = Math.round(torrent.downloadSpeed);
        if (prog !== this.lastProgress) {
          this.lastProgress = prog;
          this.emit("progress", torrent.progress);
        }
        if (spd !== this.lastSpeed) {
          this.lastSpeed = spd;
          this.emit("speed", torrent.downloadSpeed);
        }
      });

      const onMetadataOrReady = (event: "metadata" | "ready") => () => {
        if (isSettled) return; // already resolved/rejected by a previous event
        try {
          const videoFile = this.getPreferredMediaFile(torrent);
          this.emit(event, torrent, videoFile);
          this.emit("progress", torrent.progress);
          this.emit("speed", torrent.downloadSpeed);
          emitDiscoveredPeerCount();
          settleResolve(torrent);
        } catch (error) {
          const normalized = this.normalizeError(error);
          this.emit("error", normalized);
          settleReject(normalized);
        }
      };

      this.cleanup.on(torrent as unknown as TorrentEmitter, "metadata", onMetadataOrReady("metadata") as (...args: unknown[]) => void);
      this.cleanup.on(torrent as unknown as TorrentEmitter, "ready", onMetadataOrReady("ready") as (...args: unknown[]) => void);

this.cleanup.on(torrent as unknown as TorrentEmitter, "error", ((error?: Error) => {
          const normalized = this.normalizeError(error);
          this.emit("error", normalized);
          settleReject(normalized);
        }) as (...args: unknown[]) => void);
    });
  }

  private async addElectronTorrentSource(torrentSource: TorrentSource, abortSignal?: AbortSignal, expectedGeneration?: number): Promise<TorrentInstance> {
    const adapter = this.getElectronBackend();
    if (!adapter) {
      this.backendType = null;
      throw new Error("Electron torrent backend is unavailable");
    }

    try {
      const torrent =
        typeof torrentSource === "string"
          ? await adapter.addMagnet(torrentSource)
          : await adapter.addTorrentFile(torrentSource);

      if (abortSignal?.aborted) throw new Error("Torrent load was cancelled");
      if (expectedGeneration !== undefined && this.loadGeneration !== expectedGeneration) {
        throw new Error("Torrent load superseded by a newer request");
      }
      this.activeTorrent = torrent;
      this.emitTorrentStats(this.activeTorrent!);
      this.startBackendStatsPolling();
      return this.activeTorrent!;
    } catch (error) {
      const normalized = this.normalizeError(error);
      this.emit("error", normalized);
      throw normalized;
    }
  }

  /** Get the preferred video file from a torrent. */
  getVideoFile(torrent: TorrentInstance): TorrentFile {
    return this.getPreferredMediaFile(torrent).file;
  }

  /**
   * Stream a torrent file to a media element.
   * Handles native playback, mux conversion, and blob fallback.
   */
  async streamToMedia(file: TorrentFile, mediaElement: HTMLMediaElement): Promise<void> {
    this.streamAbortController?.abort();
    this.streamAbortController = new AbortController();
    this.revokeActiveObjectUrl();
    const previousFile = this.activeMediaFile?.file;
    this.activeMediaFile = this.activeTorrent
      ? this.getPlayableMediaFiles(this.activeTorrent).find((mf) => mf.file === file) ?? null
      : null;
    if (previousFile !== file) {
      this.lastBufferWindow = null;
      this.currentPlaybackBytes = 0;
    }
    mediaElement.pause();
    mediaElement.removeAttribute("src");
    mediaElement.load();

    if (file.streamUrl) {
      const isNativeSupported = this.isBrowserSupportedFormat(file.name);

      if (isNativeSupported) {
        torrentLogger.debug(`Using native browser support for: ${file.name}`);
        await this.streamWithRetry(file.streamUrl, mediaElement, file.name);
        return;
      }

      const muxBackend = this.getElectronBackend();
      if (muxBackend?.createMultiplexedStreamUrl) {
        torrentLogger.info(`Format not browser-supported, attempting mux conversion for: ${file.name}`);
        try {
          const muxUrl = await this.createMuxStreamUrl(file, null, 0);
          if (muxUrl) {
            torrentLogger.debug(`Mux conversion successful for: ${file.name}`);
            await this.streamWithRetry(muxUrl, mediaElement, file.name);
            return;
          }
        } catch (muxError) {
          torrentLogger.warn("Mux conversion failed, will try direct stream", muxError);
        }
      } else {
        torrentLogger.debug("Electron backend not available for mux conversion");
      }

      torrentLogger.info(`Falling back to direct stream for: ${file.name}`);
      await this.streamWithRetry(file.streamUrl, mediaElement, file.name);
      return;
    }

    try {
      await this.ensureStreamServer();
    } catch (error) {
      torrentLogger.warn("Stream server setup failed, falling back to blob", error instanceof Error ? error.message : String(error));
    }

    try {
      await file.streamTo!(mediaElement);
      mediaElement.load();
      return;
    } catch (error) {
      if (!file.blob) {
        throw error;
      }
      torrentLogger.warn("streamTo failed, falling back to blob", error);
    }

    // Fallback: download as blob and create object URL
    try {
      const blob = await file.blob();
      this.revokeActiveObjectUrl();
      const objectUrl = URL.createObjectURL(blob);
      this.activeObjectUrl = objectUrl;
      this.activeBlobUrls.add(objectUrl);
      mediaElement.src = objectUrl;
      mediaElement.load();
    } catch (blobError) {
      throw new Error(`Failed to load media using all available methods. Last error: ${(blobError as Error)?.message ?? String(blobError)}`);
    }
  }

  private streamFromUrl(streamUrl: string, mediaElement: HTMLMediaElement, abortSignal?: AbortSignal): Promise<void> {
    return new Promise((resolve, reject) => {
      let settled = false;
      const abortController = new AbortController();
      const { signal } = abortController;
      const cleanupListeners = () => {
        mediaElement.removeEventListener("error", onError);
        mediaElement.removeEventListener("canplay", onCanPlay);
        mediaElement.removeEventListener("loadeddata", onLoadedData);
        mediaElement.removeEventListener("loadedmetadata", onLoadedMetadata);
      };
      const onAbort = () => {
        if (!settled) {
          settled = true;
          abortController.abort();
          cleanupListeners();
          reject(new Error("Stream aborted"));
        }
      };
      abortSignal?.addEventListener("abort", onAbort, { once: true });

      const settle = (fn: () => void) => {
        if (settled) return;
        settled = true;
        abortSignal?.removeEventListener("abort", onAbort);
        abortController.abort();
        cleanupListeners();
        fn();
      };

      const onError = () => {
        const error = mediaElement.error;
        const errorMsg = error
          ? `[${error.code}] ${error.message}`
          : "network error (connection reset or truncated response — ffmpeg may have failed)";
        settle(() => reject(new Error(`Failed to load stream from URL: ${streamUrl} (${errorMsg})`)));
      };

      const onCanPlay = () => {
        settle(resolve);
      };

      const onLoadedData = () => {
        settle(resolve);
      };

      const resettableTimeout = () => {
        let id = setTimeout(() => {
          settle(() => reject(new Error(`Stream load timed out: ${streamUrl}`)));
        }, STREAM_CONFIG.streamLoadTimeoutMs);
        return {
          clear: () => clearTimeout(id),
          reset: () => { clearTimeout(id); id = setTimeout(() => {
            settle(() => reject(new Error(`Stream load timed out: ${streamUrl}`)));
          }, STREAM_CONFIG.streamLoadTimeoutMs); },
        };
      };
      const timeout = resettableTimeout();

      const onLoadedMetadata = () => {
        timeout.reset();
      };

      signal.addEventListener("abort", () => {
        timeout.clear();
        abortSignal?.removeEventListener("abort", onAbort);
        cleanupListeners();
      }, { once: true });

      mediaElement.addEventListener("error", onError, { signal });
      mediaElement.addEventListener("canplay", onCanPlay, { signal });
      mediaElement.addEventListener("loadeddata", onLoadedData, { signal });
      mediaElement.addEventListener("loadedmetadata", onLoadedMetadata, { signal });
      if (streamUrl.startsWith("http://") || streamUrl.startsWith("https://")) {
        mediaElement.crossOrigin = "anonymous";
      }
      mediaElement.src = streamUrl;
      mediaElement.load();
    });
  }

  private async streamWithRetry(streamUrl: string, mediaElement: HTMLMediaElement, fileName: string): Promise<void> {
    try {
      await this.streamFromUrl(streamUrl, mediaElement, this.streamAbortController?.signal);
    } catch (error) {
      if (this.streamAbortController?.signal.aborted) throw error;
      const isTimeout = error instanceof Error && error.message.startsWith("Stream load timed out");
      if (!isTimeout) throw error;
      torrentLogger.warn(`Stream timed out for ${fileName}, retrying once...`);
      mediaElement.removeAttribute("src");
      mediaElement.load();
      await this.streamFromUrl(streamUrl, mediaElement, this.streamAbortController?.signal);
    }
  }

  /** Format a media file's size as a human-readable string. */
  formatMediaFileLabel(mediaFile: TorrentMediaFile): string {
    return formatBytes(mediaFile.length);
  }

  /** Clear the active torrent and release all associated resources. */
  async clearActiveTorrent(): Promise<void> {
    return this.clearActiveTorrentForAdd();
  }

  async clearActiveTorrentForAdd(): Promise<void> {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    this.activeMediaFile = null;
    this.lastBufferWindow = null;
    this.lastProgress = -1;
    this.lastSpeed = -1;
    this.lastPeerCount = -1;
    this.stopPrioritizeLoop();
    this.stopBackendStatsPolling();
    this.revokeActiveObjectUrl();
    this.revokeAllBlobUrls();
    this.discoveredPeerIds.clear();

    const adapter = this.getElectronBackend();
    if (adapter) {
      try {
        await adapter.clear();
      } catch (cleanupError) {
        torrentLogger.warn("Electron backend clear failed:", cleanupError);
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
   * Clears all torrents, blob URLs, service workers, and listeners.
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
    this.backendType = null;
  }

  /**
   * Update the current playback position to prioritize downloading nearby data.
   * @param currentTimeSeconds - Current playback position in seconds.
   * @param fileLengthBytes - Total file size in bytes.
   * @param fileDurationSeconds - Total file duration in seconds.
   */
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

  /** Get the current buffer window boundaries and maximum buffer size in megabytes. */
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

  /** Immediately apply buffer priority around the current playback position. */
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

    const fileEnd = file.length - 1;

    const maxBufferHalf = this.maxBufferBytes / 2;
    const absoluteMaxStart = Math.max(0, this.currentPlaybackBytes - maxBufferHalf);
    const absoluteMaxEnd = Math.min(fileEnd, this.currentPlaybackBytes + maxBufferHalf);

    const windowStart = Math.max(0, this.currentPlaybackBytes - this.bufferWindowBytes);
    const windowEnd = Math.min(fileEnd, this.currentPlaybackBytes + this.bufferWindowBytes);

    const bufferStart = Math.max(windowStart, absoluteMaxStart);
    const bufferEnd = Math.min(windowEnd, absoluteMaxEnd);

    const prev = this.lastBufferWindow;
    if (prev) {
      if (bufferStart > prev.start) {
        const deselectEnd = Math.min(bufferStart - 1, prev.end);
        if (prev.start <= deselectEnd) {
          torrent.deselect(prev.start, deselectEnd, 0);
        }
      }
      if (bufferEnd < prev.end) {
        const deselectStart = Math.max(bufferEnd + 1, prev.start);
        if (deselectStart <= prev.end) {
          torrent.deselect(deselectStart, prev.end, 0);
        }
      }
      if (bufferStart < prev.start) {
        const selectEnd = prev.start - 1;
        if (bufferStart <= selectEnd) {
          torrent.select(bufferStart, selectEnd, 1);
        }
      }
      if (bufferEnd > prev.end) {
        const selectStart = prev.end + 1;
        if (selectStart <= bufferEnd) {
          torrent.select(selectStart, bufferEnd, 1);
        }
      }
    } else {
      torrent.deselect(0, fileEnd, 0);
      torrent.select(bufferStart, bufferEnd, 1);
    }

    this.lastBufferWindow = { start: bufferStart, end: bufferEnd };
    this.enforceBufferLimit();
  }

  private enforceBufferLimit(): void {
    const torrent = this.activeTorrent as unknown as {
      paused?: boolean;
      pause?: () => void;
      resume?: () => void;
      downloaded?: number;
    } | null;

    if (!torrent?.pause || !torrent?.resume) return;

    const downloaded = torrent.downloaded ?? 0;

    if (downloaded > this.maxBufferBytes && !torrent.paused) {
      torrent.pause();
    } else if (downloaded < this.maxBufferBytes * 0.9 && torrent.paused) {
      torrent.resume();
    }
  }

  private async getClient(): Promise<TorrentClient> {
    if (!this.client) {
      if (this.isDestroyed) {
        throw new Error("TorrentService has been destroyed");
      }
      const { default: WebTorrent } = await import("webtorrent");
      this.client = new WebTorrent({
        maxConns: TORRENT_CONFIG.maxConnections,
        tracker: { announce: getTrackerUrls() },
        sequential: true,
      }) as TorrentClient;
    }
    if (this.isDestroyed) {
      throw new Error("TorrentService has been destroyed");
    }
    return this.client;
  }

  private getElectronBackend(): ElectronTorrentBackendAdapter | null {
    const adapter = getElectronBackendAdapter();
    if (!adapter) {
      if (this.backendType === "electron") this.backendType = null;
      return null;
    }
    return adapter;
  }

  private startBackendStatsPolling(): void {
    this.stopBackendStatsPolling();
    this.statsIntervalId = this.cleanup.setInterval(() => {
      if (this.isDestroyed) {
        this.stopBackendStatsPolling();
        return;
      }
      void this.refreshBackendStats();
    }, 1000);
    void this.refreshBackendStats();
  }

  private stopBackendStatsPolling(): void {
    this.backendStatsInFlight = false;
    if (this.statsIntervalId !== null) {
      clearInterval(this.statsIntervalId);
      this.statsIntervalId = null;
    }
  }

  private async refreshBackendStats(): Promise<void> {
    const adapter = this.getElectronBackend();
    if (this.backendStatsInFlight || !adapter || !this.activeTorrent || this.isDestroyed) return;
    this.backendStatsInFlight = true;
    try {
      const snapshot = await adapter.getStats();
      if (!snapshot || !this.activeTorrent || this.isDestroyed) return;
      this.mergeTorrentSnapshot(this.activeTorrent, snapshot as unknown as TorrentInstance);
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
      if (index >= 0 && index < target.files.length) continue;
      target.files[index] = { ...snapshotFile };
    }
  }

  private emitTorrentStats(torrent: TorrentInstance): void {
    const prog = Math.round(torrent.progress * 100);
    const spd = Math.round(torrent.downloadSpeed);
    const peers = torrent.discoveredPeerCount ?? this.discoveredPeerIds.size;
    if (prog !== this.lastProgress) {
      this.lastProgress = prog;
      this.emit("progress", torrent.progress);
    }
    if (spd !== this.lastSpeed) {
      this.lastSpeed = spd;
      this.emit("speed", torrent.downloadSpeed);
    }
    if (peers !== this.lastPeerCount) {
      this.lastPeerCount = peers;
      this.emit("peerCount", peers);
    }
  }

  private streamServerRetryCount = 0;
  private readonly maxStreamServerRetries = 3;

  private async ensureStreamServer(): Promise<void> {
    if (this.streamServerReady) return;
    if (this.getElectronBackend()) return;
    if (this.streamServerPromise) return this.streamServerPromise;
    if (typeof navigator === "undefined" || !navigator?.serviceWorker?.register) return;
    if (this.streamServerRetryCount >= this.maxStreamServerRetries) return;

    this.streamServerRetryCount++;
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
        this.streamServerRetryCount = 0;
      } catch (error) {
        const normalizedError = error instanceof Error ? error : new Error(String(error));
        torrentLogger.warn("Stream server setup failed", normalizedError.message);
        this.streamServerPromise = null;
      }
    })();

    return this.streamServerPromise;
  }

  private revokeActiveObjectUrl(): void {
    if (!this.activeObjectUrl) return;
    URL.revokeObjectURL(this.activeObjectUrl);
    this.activeBlobUrls.delete(this.activeObjectUrl);
    this.activeObjectUrl = null;
  }

  private revokeAllBlobUrls(): void {
    for (const url of this.activeBlobUrls) {
      URL.revokeObjectURL(url);
    }
    this.activeBlobUrls.clear();
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
    if (typeof error === "string") return new Error(error);
    if (typeof error === "object") {
      const errObj = error as Record<string, unknown>;
      const message = typeof errObj.message === "string" ? errObj.message : "Unknown torrent error";
      const normalized = new Error(message);
      if (typeof errObj.code === "string" || typeof errObj.code === "number") {
        (normalized as Error & { code?: string | number }).code = errObj.code;
      }
      if (typeof errObj.stack === "string") {
        normalized.stack = errObj.stack;
      }
      return normalized;
    }
    return new Error("Unknown torrent error");
  }
}

export default TorrentService;
