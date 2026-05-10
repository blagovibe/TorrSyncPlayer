import WebTorrent from "webtorrent";

interface TorrentFile {
  name: string;
  streamTo: (videoElement: HTMLVideoElement) => Promise<void>;
}

interface TorrentInstance {
  files: TorrentFile[];
  progress: number;
  downloadSpeed: number;
  on: (event: "download" | "ready" | "error", callback: (error?: Error) => void) => void;
  destroy?: (callback?: (error?: Error) => void) => void;
}

type TorrentEvents = {
  progress: (progress: number) => void;
  speed: (bytesPerSecond: number) => void;
  ready: (torrent: TorrentInstance, videoFile: TorrentFile) => void;
  error: (error: Error) => void;
};

type EventKey = keyof TorrentEvents;

export class TorrentService {
  private client: { add: (magnetLink: string) => TorrentInstance; destroy: () => void };
  private activeTorrent: TorrentInstance | null = null;
  private listeners: { [K in EventKey]: Set<TorrentEvents[K]> } = {
    progress: new Set(),
    speed: new Set(),
    ready: new Set(),
    error: new Set(),
  };

  constructor() {
    this.client = new WebTorrent();
  }

  on<K extends EventKey>(event: K, callback: TorrentEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  async addMagnet(magnetLink: string): Promise<TorrentInstance> {
    this.clearActiveTorrent();

    return new Promise<TorrentInstance>((resolve, reject) => {
      const torrent = this.client.add(magnetLink);
      this.activeTorrent = torrent;

      torrent.on("download", () => {
        this.emit("progress", torrent.progress);
        this.emit("speed", torrent.downloadSpeed);
      });

      torrent.on("ready", async () => {
        try {
          const videoFile = this.getVideoFile(torrent);
          this.emit("ready", torrent, videoFile);
          resolve(torrent);
        } catch (error) {
          const normalized = this.normalizeError(error);
          this.emit("error", normalized);
          reject(normalized);
        }
      });

      torrent.on("error", (error?: Error) => {
        const normalized = this.normalizeError(error);
        this.emit("error", normalized);
        reject(normalized);
      });
    });
  }

  getVideoFile(torrent: TorrentInstance): TorrentFile {
    const videoExtensions = [".mp4", ".webm", ".mkv"];
    const videoFile = torrent.files.find((file) =>
      videoExtensions.some((ext) => file.name.toLowerCase().endsWith(ext)),
    );

    if (!videoFile) {
      throw new Error("No supported video file found in torrent");
    }

    return videoFile;
  }

  async streamToVideo(file: TorrentFile, videoElement: HTMLVideoElement): Promise<void> {
    videoElement.removeAttribute("src");
    videoElement.load();
    await file.streamTo(videoElement);
  }

  clearActiveTorrent(): void {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    if (torrent?.destroy) {
      torrent.destroy();
    }
  }

  destroy(): void {
    this.clearActiveTorrent();
    this.client.destroy();
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<TorrentEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<TorrentEvents[K]>) => void)(...args);
    }
  }

  private normalizeError(error: unknown): Error {
    if (error instanceof Error) {
      return error;
    }
    return new Error(typeof error === "string" ? error : "Unknown torrent error");
  }
}

export default TorrentService;
