import SignalingService from "./SignalingService";
import { SyncMessage } from "./types";

type SyncEvents = {
  sync_play: (message: SyncMessage) => void;
  sync_pause: (message: SyncMessage) => void;
  sync_seek: (message: SyncMessage) => void;
  outbound_sync: (message: SyncMessage) => void;
};

type EventKey = keyof SyncEvents;
type Role = "master" | "slave";

export class SyncService {
  private readonly signaling: Pick<SignalingService, "sendSync">;
  private readonly video: HTMLVideoElement;
  private readonly role: Role;
  private readonly cleanups: Array<() => void> = [];
  private listeners: { [K in EventKey]: Set<SyncEvents[K]> } = {
    sync_play: new Set(),
    sync_pause: new Set(),
    sync_seek: new Set(),
    outbound_sync: new Set(),
  };
  private suppressNextEventSync: Partial<Record<"play" | "pause" | "seeked", boolean>> = {};

  constructor(signaling: Pick<SignalingService, "sendSync">, video: HTMLVideoElement, role: Role) {
    this.signaling = signaling;
    this.video = video;
    this.role = role;

    if (this.role === "master") {
      this.bindMasterEvents();
    }
  }

  on<K extends EventKey>(event: K, callback: SyncEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  dispose(): void {
    for (const cleanup of this.cleanups) {
      cleanup();
    }
    this.cleanups.length = 0;
    for (const callbacks of Object.values(this.listeners)) {
      callbacks.clear();
    }
  }

  play(): void {
    void this.video.play();
    if (this.role === "master") {
      this.suppressNextEventSync.play = true;
      this.sendMasterSync("play", this.video.currentTime);
    }
  }

  pause(): void {
    this.video.pause();
    if (this.role === "master") {
      this.suppressNextEventSync.pause = true;
      this.sendMasterSync("pause", this.video.currentTime);
    }
  }

  seek(timestamp: number): void {
    this.video.currentTime = timestamp;
    if (this.role === "master") {
      this.suppressNextEventSync.seeked = true;
      this.sendMasterSync("seek", timestamp);
    }
  }

  getCurrentTime(): number {
    return this.video.currentTime;
  }

  applyRemoteSync(message: SyncMessage): void {
    if (this.role !== "slave") {
      return;
    }

    const latencySeconds = Math.max((Date.now() - message.server_ts) / 1000, 0);
    const compensatedPosition = message.position + latencySeconds;

    switch (message.action) {
      case "play":
        this.video.currentTime = compensatedPosition;
        void this.video.play();
        this.emit("sync_play", message);
        break;
      case "pause":
        this.video.currentTime = compensatedPosition;
        this.video.pause();
        this.emit("sync_pause", message);
        break;
      case "seek":
        this.video.currentTime = compensatedPosition;
        this.emit("sync_seek", message);
        break;
    }
  }

  private bindMasterEvents(): void {
    const onPlay = () => {
      if (this.suppressNextEventSync.play) {
        this.suppressNextEventSync.play = false;
        return;
      }
      this.sendMasterSync("play", this.video.currentTime);
    };
    const onPause = () => {
      if (this.suppressNextEventSync.pause) {
        this.suppressNextEventSync.pause = false;
        return;
      }
      this.sendMasterSync("pause", this.video.currentTime);
    };
    const onSeeked = () => {
      if (this.suppressNextEventSync.seeked) {
        this.suppressNextEventSync.seeked = false;
        return;
      }
      this.sendMasterSync("seek", this.video.currentTime);
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

  private sendMasterSync(action: SyncMessage["action"], position: number): void {
    this.signaling.sendSync(action, position);
    this.emit("outbound_sync", {
      action,
      position,
      server_ts: Date.now(),
    });
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<SyncEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<SyncEvents[K]>) => void)(...args);
    }
  }
}

export default SyncService;
