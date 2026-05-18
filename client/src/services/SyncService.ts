import { type SyncMessage } from "./types";
import { createCleanup, type CleanupHandle } from "../utils/cleanup";
import { SYNC_CONFIG } from "../config";

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
  private readonly suppressNextEventSync: Partial<Record<"play" | "pause" | "seeked", boolean>> = {};
  private syncToleranceSeconds = SYNC_CONFIG.defaultToleranceSeconds;
  private isDisposed = false;

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

  on<K extends EventKey>(event: K, callback: SyncEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  dispose(): void {
    this.isDisposed = true;
    this.cleanup.abort();
    for (const key of Object.keys(this.listeners) as EventKey[]) {
      this.listeners[key].clear();
    }
  }

  setSyncToleranceSeconds(value: number): void {
    this.syncToleranceSeconds = this.normalizeTolerance(value);
  }

  getSyncToleranceSeconds(): number {
    return this.syncToleranceSeconds;
  }

  createSnapshot(): SyncMessage {
    return {
      action: "state",
      position: this.video.currentTime,
      server_ts: Date.now(),
      is_playing: !this.video.paused,
    };
  }

  play(): void {
    if (this.isDisposed) return;
    this.video.play().catch(() => {
      // Autoplay may be blocked — user interaction required
    });
    if (this.role === "master") {
      this.suppressNextEventSync.play = true;
      this.sendMasterSync("play", this.video.currentTime, true);
    }
  }

  pause(): void {
    if (this.isDisposed) return;
    this.video.pause();
    if (this.role === "master") {
      this.suppressNextEventSync.pause = true;
      this.sendMasterSync("pause", this.video.currentTime, false);
    }
  }

  seek(timestamp: number): void {
    if (this.isDisposed) return;
    this.video.currentTime = timestamp;
    if (this.role === "master") {
      this.suppressNextEventSync.seeked = true;
      this.sendMasterSync("seek", timestamp, !this.video.paused);
    }
  }

  getCurrentTime(): number {
    return this.video.currentTime;
  }

  applyRemoteSync(message: SyncMessage): void {
    if (this.role !== "slave" || this.isDisposed) return;

    const isSeek = message.action === "seek";
    const latencySeconds = isSeek
      ? 0
      : Math.min(Math.max((Date.now() - message.server_ts) / 1000, 0), SYNC_CONFIG.maxLatencyCompensationSeconds);
    const compensatedPosition = message.position + latencySeconds;
    const shouldAlign =
      Math.abs(this.video.currentTime - compensatedPosition) > this.syncToleranceSeconds;
    const desiredPlayState =
      message.is_playing ??
      (message.action === "play" || message.action === "seek" || message.action === "state");
    const isPaused = this.video.paused;

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

    switch (message.action) {
      case "play":
        if (desiredPlayState && isPaused) {
          safePlay();
        } else if (!desiredPlayState && !isPaused) {
          this.video.pause();
        }
        this.emit("sync_play", message);
        break;
      case "pause":
        if (desiredPlayState && isPaused) {
          safePlay();
        } else if (!desiredPlayState && !isPaused) {
          this.video.pause();
        }
        this.emit("sync_pause", message);
        break;
      case "seek":
        if (desiredPlayState && isPaused) {
          safePlay();
        } else if (!desiredPlayState && !isPaused) {
          this.video.pause();
        }
        this.emit("sync_seek", message);
        break;
      case "state":
        if (desiredPlayState && isPaused) {
          safePlay();
        } else if (!desiredPlayState && !isPaused) {
          this.video.pause();
        }
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

  private startHeartbeat(): void {
    this.cleanup.setInterval(() => {
      if (this.role !== "master" || this.isDisposed) return;

      // Reset suppression flags to prevent stale state
      if (this.suppressNextEventSync.seeked) this.suppressNextEventSync.seeked = false;
      if (this.suppressNextEventSync.play) this.suppressNextEventSync.play = false;
      if (this.suppressNextEventSync.pause) this.suppressNextEventSync.pause = false;

      this.sendMasterSync("state", this.video.currentTime, !this.video.paused);
    }, SYNC_CONFIG.heartbeatIntervalMs);
  }

  private sendMasterSync(
    action: SyncMessage["action"],
    position: number,
    isPlaying: boolean,
  ): void {
    if (this.isDisposed) return;
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
