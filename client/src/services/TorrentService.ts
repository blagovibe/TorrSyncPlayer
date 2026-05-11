type MediaKind = "video" | "audio";

interface TorrentFile {
  name: string;
  length?: number;
  streamTo: (mediaElement: HTMLMediaElement) => Promise<void>;
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
  on: (event: "download" | "ready" | "error", callback: (error?: Error) => void) => void;
  destroy?: (callback?: (error?: Error) => void) => void;
}

type TorrentEvents = {
  progress: (progress: number) => void;
  speed: (bytesPerSecond: number) => void;
  ready: (torrent: TorrentInstance, mediaFile: TorrentMediaFile) => void;
  error: (error: Error) => void;
};

type EventKey = keyof TorrentEvents;
type TorrentSource = string | Uint8Array;
type TorrentClient = { add: (torrentSource: TorrentSource) => TorrentInstance; destroy: () => void };

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

function getFileExtension(name: string): string {
  const normalized = name.trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) {
    return "";
  }
  return normalized.slice(lastDot);
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return "Unknown size";
  }

  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

export class TorrentService {
  private client: TorrentClient | null = null;
  private activeTorrent: TorrentInstance | null = null;
  private listeners: { [K in EventKey]: Set<TorrentEvents[K]> } = {
    progress: new Set(),
    speed: new Set(),
    ready: new Set(),
    error: new Set(),
  };

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

  private async addTorrentSource(torrentSource: TorrentSource): Promise<TorrentInstance> {
    this.clearActiveTorrent();
    const client = await this.getClient();

    return new Promise<TorrentInstance>((resolve, reject) => {
      const torrent = client.add(torrentSource);
      this.activeTorrent = torrent;

      torrent.on("download", () => {
        this.emit("progress", torrent.progress);
        this.emit("speed", torrent.downloadSpeed);
      });

      torrent.on("ready", async () => {
        try {
          const videoFile = this.getPreferredMediaFile(torrent);
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
    return this.getPreferredMediaFile(torrent).file;
  }

  async streamToMedia(file: TorrentFile, mediaElement: HTMLMediaElement): Promise<void> {
    mediaElement.pause();
    mediaElement.removeAttribute("src");
    mediaElement.load();
    await file.streamTo(mediaElement);
  }

  formatMediaFileLabel(mediaFile: TorrentMediaFile): string {
    return formatBytes(mediaFile.length);
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
    this.client?.destroy();
  }

  private async getClient(): Promise<TorrentClient> {
    if (!this.client) {
      const { default: WebTorrent } = await import("webtorrent");
      this.client = new WebTorrent() as TorrentClient;
    }
    return this.client;
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
