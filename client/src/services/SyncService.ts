import { type SyncMessage } from "./types";

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

const DEFAULT_SYNC_TOLERANCE_SECONDS = 0.5;
const HEARTBEAT_INTERVAL_MS = 1000;

export class SyncService {
  private readonly signaling: SyncTransport;
  private readonly video: HTMLVideoElement;
  private readonly role: Role;
  private readonly cleanups: Array<() => void> = [];
  private readonly listeners: { [K in EventKey]: Set<SyncEvents[K]> } = {
    sync_play: new Set(),
    sync_pause: new Set(),
    sync_seek: new Set(),
    sync_state: new Set(),
    outbound_sync: new Set(),
  };
  private readonly suppressNextEventSync: Partial<Record<"play" | "pause" | "seeked", boolean>> = {};
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private syncToleranceSeconds = DEFAULT_SYNC_TOLERANCE_SECONDS;

  constructor(
    signaling: SyncTransport,
    video: HTMLVideoElement,
    role: Role,
    syncToleranceSeconds = DEFAULT_SYNC_TOLERANCE_SECONDS,
  ) {
    this.signaling = signaling;
    this.video = video;
    this.role = role;
    this.syncToleranceSeconds = this.normalizeTolerance(syncToleranceSeconds);

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
    this.stopHeartbeat();
    for (const cleanup of this.cleanups) {
      cleanup();
    }
    this.cleanups.length = 0;
    for (const callbacks of Object.values(this.listeners)) {
      callbacks.clear();
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
    void this.video.play().catch(() => undefined);
    if (this.role === "master") {
      this.suppressNextEventSync.play = true;
      this.sendMasterSync("play", this.video.currentTime, true);
    }
  }

  pause(): void {
    this.video.pause();
    if (this.role === "master") {
      this.suppressNextEventSync.pause = true;
      this.sendMasterSync("pause", this.video.currentTime, false);
    }
  }

  seek(timestamp: number): void {
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
    if (this.role !== "slave") {
      return;
    }

    // For seek messages, don't apply latency compensation — the master
    // already computed the target position and we need to jump there
    // immediately. Latency compensation is only useful for continuous
    // playback (play/pause/state).
    const isSeek = message.action === "seek";
    const latencySeconds = isSeek
      ? 0
      : Math.min(Math.max((Date.now() - message.server_ts) / 1000, 0), 5);
    const compensatedPosition = message.position + latencySeconds;
    const shouldAlign = Math.abs(this.video.currentTime - compensatedPosition) > this.syncToleranceSeconds;
    const desiredPlayState =
      message.is_playing ??
      (message.action === "play" || message.action === "seek" || message.action === "state");
    const isPaused = this.video.paused;

    if (shouldAlign) {
      this.video.currentTime = compensatedPosition;
    }

    // Helper: try to play only if we have enough buffered data.
    // HAVE_CURRENT_DATA (2) means we have data for the current frame.
    const safePlay = () => {
      if (this.video.readyState >= 2) {
        void this.video.play().catch(() => undefined);
      }
      // If readyState < 2, the browser will auto-play once enough data
      // is buffered — no need to spam play() calls.
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

    this.video.addEventListener("play", onPlay);
    this.video.addEventListener("pause", onPause);
    this.video.addEventListener("seeked", onSeeked);
    this.cleanups.push(
      () => this.video.removeEventListener("play", onPlay),
      () => this.video.removeEventListener("pause", onPause),
      () => this.video.removeEventListener("seeked", onSeeked),
    );
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = globalThis.setInterval(() => {
      if (this.role !== "master") {
        return;
      }

      // Safety: if a seek was suppressed but 'seeked' never fired
      // (e.g. seek to same position), reset the flag so future
      // seeked events are not permanently suppressed.
      if (this.suppressNextEventSync.seeked) {
        this.suppressNextEventSync.seeked = false;
      }

      this.sendMasterSync("state", this.video.currentTime, !this.video.paused);
    }, HEARTBEAT_INTERVAL_MS);
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer !== null) {
      globalThis.clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  private sendMasterSync(action: SyncMessage["action"], position: number, isPlaying: boolean): void {
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
    if (!Number.isFinite(value) || value < 0) {
      return DEFAULT_SYNC_TOLERANCE_SECONDS;
    }
    return value;
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<SyncEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<SyncEvents[K]>) => void)(...args);
    }
  }
}

export default SyncService;
