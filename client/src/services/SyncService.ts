import SignalingService from "./SignalingService";
import { SyncMessage } from "./types";

type SyncEvents = {
  sync_play: (message: SyncMessage) => void;
  sync_pause: (message: SyncMessage) => void;
  sync_seek: (message: SyncMessage) => void;
};

type EventKey = keyof SyncEvents;
type Role = "master" | "slave";

export class SyncService {
  private readonly signaling: SignalingService;
  private readonly video: HTMLVideoElement;
  private readonly role: Role;
  private listeners: { [K in EventKey]: Set<SyncEvents[K]> } = {
    sync_play: new Set(),
    sync_pause: new Set(),
    sync_seek: new Set(),
  };

  constructor(signaling: SignalingService, video: HTMLVideoElement, role: Role) {
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

  play(): void {
    void this.video.play();
    if (this.role === "master") {
      this.signaling.sendSync("play", this.video.currentTime);
    }
  }

  pause(): void {
    this.video.pause();
    if (this.role === "master") {
      this.signaling.sendSync("pause", this.video.currentTime);
    }
  }

  seek(timestamp: number): void {
    this.video.currentTime = timestamp;
    if (this.role === "master") {
      this.signaling.sendSync("seek", timestamp);
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
    this.video.addEventListener("play", () => {
      this.signaling.sendSync("play", this.video.currentTime);
    });
    this.video.addEventListener("pause", () => {
      this.signaling.sendSync("pause", this.video.currentTime);
    });
    this.video.addEventListener("seeked", () => {
      this.signaling.sendSync("seek", this.video.currentTime);
    });
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<SyncEvents[K]>) {
    for (const callback of this.listeners[event]) {
      callback(...args);
    }
  }
}

export default SyncService;
