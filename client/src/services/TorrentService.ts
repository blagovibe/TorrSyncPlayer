import WebTorrent, { File, Torrent } from "webtorrent";

type TorrentEvents = {
  progress: (progress: number) => void;
  ready: (torrent: Torrent, videoFile: File) => void;
  error: (error: Error) => void;
};

type EventKey = keyof TorrentEvents;

export class TorrentService {
  private client: WebTorrent.Instance;
  private listeners: { [K in EventKey]: Set<TorrentEvents[K]> } = {
    progress: new Set(),
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

  async addMagnet(magnetLink: string): Promise<Torrent> {
    return new Promise<Torrent>((resolve, reject) => {
      const torrent = this.client.add(magnetLink);

      torrent.on("download", () => {
        this.emit("progress", torrent.progress);
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

      torrent.on("error", (error: Error) => {
        this.emit("error", error);
        reject(error);
      });
    });
  }

  getVideoFile(torrent: Torrent): File {
    const videoExtensions = [".mp4", ".webm", ".mkv"];
    const videoFile = torrent.files.find((file) =>
      videoExtensions.some((ext) => file.name.toLowerCase().endsWith(ext)),
    );

    if (!videoFile) {
      throw new Error("No supported video file found in torrent");
    }

    return videoFile;
  }

  async streamToVideo(file: File, videoElement: HTMLVideoElement): Promise<void> {
    await file.streamTo(videoElement);
  }

  destroy(): void {
    this.client.destroy();
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<TorrentEvents[K]>) {
    for (const callback of this.listeners[event]) {
      callback(...args);
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
