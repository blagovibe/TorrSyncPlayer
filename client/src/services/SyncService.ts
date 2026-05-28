import { type SyncMessage } from "./types";
import { createCleanup, type CleanupHandle } from "../utils/cleanup";
import { SYNC_CONFIG } from "../config-unified";

type SyncEvents = {
  sync_play: (message: SyncMessage) => void;
  sync_pause: (message: SyncMessage) => void;
  sync_seek: (message: SyncMessage) => void;
  sync_state: (message: SyncMessage) => void;
  outbound_sync: (message: SyncMessage) => void;
};

type EventKey = keyof SyncEvents;
type Role = "master" | "slave";

type SyncTransport = {
  sendSync: (message: SyncMessage) => void;
};

type SuppressFlags = {
  play: boolean;
  pause: boolean;
  seeked: boolean;
  state: boolean;
};

/**
 * Service for synchronizing playback between peers.
 *
 * Handles play/pause/seek events and ensures all peers stay in sync.
 * The master peer broadcasts state changes, while slave peers apply them.
 *
 * @example
 * ```typescript
 * const sync = new SyncService(p2pService, videoElement, "master");
 * sync.on("sync_play", (msg) => console.log("Play at", msg.position));
 * sync.play(); // Broadcasts play event to all peers
 * ```
 */
export class SyncService {
  private readonly signaling: SyncTransport;
  private readonly video: HTMLVideoElement;
  private readonly role: Role;
  private readonly cleanup: CleanupHandle;
  private readonly listeners: { [K in EventKey]: Set<SyncEvents[K]> } = {
    sync_play: new Set(),
    sync_pause: new Set(),
    sync_seek: new Set(),
    sync_state: new Set(),
    outbound_sync: new Set(),
  };
  private suppressNextEventSync: SuppressFlags = {
    play: false,
    pause: false,
    seeked: false,
    state: false,
  };
  private lastExplicitSyncTs = 0;
  private syncToleranceSeconds = SYNC_CONFIG.defaultToleranceSeconds;
  private disposed = false;

  constructor(
    signaling: SyncTransport,
    video: HTMLVideoElement,
    role: Role,
    syncToleranceSeconds = SYNC_CONFIG.defaultToleranceSeconds,
  ) {
    this.signaling = signaling;
    this.video = video;
    this.role = role;
    this.syncToleranceSeconds = this.normalizeTolerance(syncToleranceSeconds);
    this.cleanup = createCleanup();

    if (this.role === "master") {
      this.bindMasterEvents();
      this.startHeartbeat();
    }
  }

  /**
   * Register an event listener.
   * @returns An unsubscribe function.
   */
  on<K extends EventKey>(event: K, callback: SyncEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  /**
   * Dispose of the sync service and clean up all event listeners.
   * After calling this, the service should not be used.
   */
  dispose(): void {
    this.disposed = true;
    this.stopHeartbeat();
    this.cleanup.abort();
    this.lastExplicitSyncTs = 0;
    for (const key of Object.keys(this.listeners) as EventKey[]) {
      this.listeners[key].clear();
    }
  }

  /**
   * Set the sync tolerance in seconds. Values are normalized to non-negative.
   * @param value - The tolerance in seconds.
   */
  setSyncToleranceSeconds(value: number): void {
    this.syncToleranceSeconds = this.normalizeTolerance(value);
  }

  /**
   * Get the current sync tolerance in seconds.
   * @returns The tolerance in seconds.
   */
  getSyncToleranceSeconds(): number {
    return this.syncToleranceSeconds;
  }

  /**
   * Create a sync message snapshot of the current playback state.
   * @param sourceKey - Optional source key to identify the torrent.
   * @returns A sync message representing the current state.
   */
  createSnapshot(sourceKey?: string): SyncMessage {
    const message: SyncMessage = {
      action: "state",
      position: this.video.currentTime,
      server_ts: Date.now(),
      is_playing: !this.video.paused,
    };
    if (sourceKey) {
      message.sourceKey = sourceKey;
    }
    return message;
  }

  /**
   * Start playback on the master and broadcast a sync message.
   * On slave peers, this only starts local playback.
   */
  play(): void {
    if (this.role === "master") {
      this.suppressNextEventSync.play = true;
      this.suppressNextEventSync.state = true;
      this.lastExplicitSyncTs = Date.now();
      this.sendMasterSync("play", this.video.currentTime, true);
    }
    this.video.play().catch(() => {
      if (this.role === "master") {
        this.suppressNextEventSync.play = false;
        this.suppressNextEventSync.state = false;
      }
    });
  }

  /**
   * Pause playback on the master and broadcast a sync message.
   * On slave peers, this only pauses local playback.
   */
  pause(): void {
    try {
      this.video.pause();
    } catch {
      // pause() should not throw, but guard for spec compliance
    }
    if (this.role === "master") {
      this.suppressNextEventSync.pause = true;
      this.suppressNextEventSync.state = true;
      this.lastExplicitSyncTs = Date.now();
      this.sendMasterSync("pause", this.video.currentTime, false);
    }
  }

  /**
   * Seek to a timestamp on the master and broadcast a sync message.
   * @param timestamp - The position to seek to in seconds.
   */
  seek(timestamp: number): void {
    const wasPlaying = !this.video.paused;
    if (this.role === "master") {
      this.suppressNextEventSync.seeked = true;
      this.suppressNextEventSync.state = true;
      this.lastExplicitSyncTs = Date.now();
      this.video.currentTime = timestamp;
      this.sendMasterSync("seek", timestamp, wasPlaying);
    } else {
      this.video.currentTime = timestamp;
    }
  }

  /**
   * Get the current playback time.
   * @returns The current time in seconds.
   */
  getCurrentTime(): number {
    return this.video.currentTime;
  }

  /**
   * Apply a remote sync message to the local video element (slave only).
   * Compensates for network latency and enforces sync tolerance.
   */
  applyRemoteSync(message: SyncMessage, transportRttMs?: number | null): void {
    if (this.role !== "slave") return;

    const snapshot = {
      readyState: this.video.readyState,
      currentTime: this.video.currentTime,
      duration: this.video.duration,
      paused: this.video.paused,
    };

    if (snapshot.readyState === 0) {
      if (message.action !== "seek") {
        return;
      }
      if (!Number.isFinite(snapshot.duration) || snapshot.duration <= 0) {
        return;
      }
    }

    const isSeek = message.action === "seek";
    let latencySeconds: number;
    if (isSeek) {
      latencySeconds = 0;
    } else if (transportRttMs !== null && transportRttMs !== undefined && transportRttMs > 0) {
      latencySeconds = Math.min(transportRttMs / 1000, SYNC_CONFIG.maxLatencyCompensationSeconds);
    } else {
      latencySeconds = Math.min(Math.max((Date.now() - message.server_ts) / 1000, 0), SYNC_CONFIG.maxLatencyCompensationSeconds);
    }
    const rawCompensatedPosition = message.position + latencySeconds;
    if (!Number.isFinite(rawCompensatedPosition)) return;
    const maxDuration = Number.isFinite(snapshot.duration) && snapshot.duration > 0 ? snapshot.duration : Infinity;
    const compensatedPosition = Math.max(0, Math.min(rawCompensatedPosition, maxDuration));
    if (!Number.isFinite(compensatedPosition)) return;
    const shouldAlign =
      Math.abs(snapshot.currentTime - compensatedPosition) > this.syncToleranceSeconds;
    const desiredPlayState =
      message.is_playing ??
      (message.action === "play" || message.action === "seek" || message.action === "state");

    if (shouldAlign) {
      this.video.currentTime = compensatedPosition;
    }

    const safePlay = () => {
      if (this.video.paused && this.video.readyState >= 1) {
        this.video.play().catch(() => {
          // Autoplay may be blocked
        });
      }
    };

    const applyState = () => {
      if (desiredPlayState && snapshot.paused) {
        safePlay();
      } else if (!desiredPlayState && !snapshot.paused) {
        this.video.pause();
      }
    };

    switch (message.action) {
      case "play":
        applyState();
        this.emit("sync_play", message);
        break;
      case "pause":
        applyState();
        this.emit("sync_pause", message);
        break;
      case "seek":
        applyState();
        this.emit("sync_seek", message);
        break;
      case "state":
        applyState();
        this.emit("sync_state", message);
        break;
    }
  }

  private bindMasterEvents(): void {
    const onPlay = () => {
      if (this.suppressNextEventSync.play) {
        this.suppressNextEventSync.play = false;
        return;
      }
      this.sendMasterSync("play", this.video.currentTime, true);
    };
    const onPause = () => {
      if (this.suppressNextEventSync.pause) {
        this.suppressNextEventSync.pause = false;
        return;
      }
      this.sendMasterSync("pause", this.video.currentTime, false);
    };
    const onSeeked = () => {
      if (this.suppressNextEventSync.seeked) {
        this.suppressNextEventSync.seeked = false;
        return;
      }
      this.sendMasterSync("seek", this.video.currentTime, !this.video.paused);
    };

    this.cleanup.addEventListener(this.video, "play", onPlay);
    this.cleanup.addEventListener(this.video, "pause", onPause);
    this.cleanup.addEventListener(this.video, "seeked", onSeeked);
  }

  private lastHeartbeatPosition = -1;
  private lastHeartbeatPlaying = false;
  private lastHeartbeatSent = 0;

  private heartbeatIntervalId: ReturnType<typeof setInterval> | null = null;
  private heartbeatIdleSince: number | null = null;

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatIdleSince = null;
    this.heartbeatIntervalId = this.cleanup.setInterval(() => {
      if (this.disposed || this.role !== "master" || this.cleanup.aborted) {
        this.stopHeartbeat();
        return;
      }

      const now = Date.now();
      const position = this.video.currentTime;
      const isPlaying = !this.video.paused;

      if (this.suppressNextEventSync.state) {
        this.suppressNextEventSync.state = false;
        this.heartbeatIdleSince = null;
        return;
      }

      const timeSinceExplicitSync = now - this.lastExplicitSyncTs;
      if (timeSinceExplicitSync < SYNC_CONFIG.heartbeatIntervalMs) {
        this.heartbeatIdleSince = null;
        return;
      }

      const posChanged = Math.abs(position - this.lastHeartbeatPosition) > 0.5;
      const stateChanged = isPlaying !== this.lastHeartbeatPlaying;
      const timeSinceLastSend = now - this.lastHeartbeatSent;
      const minSyncInterval = SYNC_CONFIG.heartbeatIntervalMs;

      if (timeSinceLastSend < minSyncInterval) return;

      if (!posChanged && !stateChanged) {
        if (this.heartbeatIdleSince === null) {
          this.heartbeatIdleSince = now;
        }
        if (now - this.heartbeatIdleSince < 30_000) {
          this.sendMasterSync("state", position, isPlaying);
          this.lastHeartbeatSent = now;
        }
        return;
      }

      this.heartbeatIdleSince = null;
      this.lastHeartbeatPosition = position;
      this.lastHeartbeatPlaying = isPlaying;
      this.lastHeartbeatSent = now;
      this.sendMasterSync("state", position, isPlaying);
    }, SYNC_CONFIG.heartbeatIntervalMs);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatIntervalId !== null) {
      clearInterval(this.heartbeatIntervalId);
      this.heartbeatIntervalId = null;
    }
  }

  private sendMasterSync(
    action: SyncMessage["action"],
    position: number,
    isPlaying: boolean,
  ): void {
    const message: SyncMessage = {
      action,
      position,
      server_ts: Date.now(),
      is_playing: isPlaying,
    };
    this.signaling.sendSync(message);
    this.emit("outbound_sync", message);
  }

  private normalizeTolerance(value: number): number {
    if (!Number.isFinite(value) || value < 0) return SYNC_CONFIG.defaultToleranceSeconds;
    return value;
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<SyncEvents[K]>): void {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<SyncEvents[K]>) => void)(...args);
    }
  }
}

export default SyncService;
