import { afterEach, describe, expect, it, vi } from "vitest";
import { TorrentService } from "../TorrentService";
import { setupElectronBackendCleanup, createTorrent } from "./test-utils";
const { addMock, destroyMock, WT } = vi.hoisted(() => {
  const add = vi.fn();
  const createServer = vi.fn();
  const destroy = vi.fn();
  const WT = vi.fn(() => ({ add, createServer, destroy }));
  return { addMock: add, destroyMock: destroy, WT };
});

vi.mock("webtorrent", () => ({
  default: WT,
}));

describe("TorrentService", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    setupElectronBackendCleanup();
  });

  it("emits progress and resolves with the first supported video file when metadata is ready", async () => {
    const torrent = createTorrent([{ name: "notes.txt" }, { name: "Movie.MP4" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const progress = vi.fn();
    const speed = vi.fn();
    const metadata = vi.fn();
    const ready = vi.fn();
    const peerCount = vi.fn();

    service.on("progress", progress);
    service.on("speed", speed);
    service.on("metadata", metadata);
    service.on("ready", ready);
    service.on("peerCount", peerCount);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("metadata");
    await torrent.emit("download");
    await torrent.emit("peer", "198.51.100.7:6881");
    await torrent.emit("ready");

    await expect(result).resolves.toBe(torrent);
    expect(progress).toHaveBeenCalledWith(0.35);
    expect(speed).toHaveBeenCalledWith(2048);
    expect(metadata).toHaveBeenCalledWith(
      torrent,
      expect.objectContaining({
        name: "Movie.MP4",
        kind: "video",
      }),
    );
    expect(ready).not.toHaveBeenCalled();
    expect(peerCount).toHaveBeenCalledWith(0);
    expect(peerCount).toHaveBeenCalledWith(1);
    expect(peerCount).not.toHaveBeenCalledWith(2);
  });

  it("returns playable video and audio files in priority order", () => {
    const torrent = createTorrent([
      { name: "song.flac", length: 5_000_000 },
      { name: "clip.webm", length: 2_000_000 },
      { name: "README.txt" },
    ]);
    const service = new TorrentService();

    const playableFiles = service.getPlayableMediaFiles(torrent);

    expect(playableFiles).toHaveLength(2);
    expect(playableFiles[0]).toEqual(
      expect.objectContaining({ name: "clip.webm", kind: "video" }),
    );
    expect(playableFiles[1]).toEqual(
      expect.objectContaining({ name: "song.flac", kind: "audio" }),
    );
    expect(service.getPreferredMediaFile(torrent)).toEqual(expect.objectContaining({ name: "clip.webm" }));
  });

  it("prefers browser-friendly video containers when multiple videos are available", () => {
    const torrent = createTorrent([
      { name: "feature.mkv", length: 12_000_000 },
      { name: "feature.mp4", length: 11_000_000 },
      { name: "feature.webm", length: 10_000_000 },
    ]);
    const service = new TorrentService();

    const playableFiles = service.getPlayableMediaFiles(torrent);

    expect(playableFiles[0]).toEqual(expect.objectContaining({ name: "feature.mp4", kind: "video" }));
    expect(service.getPreferredMediaFile(torrent)).toEqual(
      expect.objectContaining({ name: "feature.mp4", kind: "video" }),
    );
  });

  it("rejects and emits an error when a metadata-ready torrent has no supported media file", async () => {
    const torrent = createTorrent([{ name: "archive.zip" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const error = vi.fn();
    service.on("error", error);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("metadata");

    await expect(result).rejects.toThrow("No supported video or audio file found in torrent");
    expect(error).toHaveBeenCalledWith(
      expect.objectContaining({ message: expect.stringContaining("No supported video or audio file found in torrent") }),
    );
  });

  it("waits for torrent cleanup callbacks before allowing the next load", async () => {
    let releaseCleanup: () => void = () => undefined;
    const destroy = vi.fn((callback?: () => void) => {
      releaseCleanup = callback ?? (() => undefined);
    });
    const service = new TorrentService();
    (service as unknown as { activeTorrent: { destroy: typeof destroy } }).activeTorrent = {
      destroy,
    } as unknown as { destroy: typeof destroy };

    let resolved = false;
    const cleanupPromise = service.clearActiveTorrentForAdd().then(() => {
      resolved = true;
    });

    await Promise.resolve();
    expect(resolved).toBe(false);

    releaseCleanup();
    await cleanupPromise;

    expect(destroy).toHaveBeenCalledOnce();
    expect(resolved).toBe(true);
  });

  it("normalizes torrent errors", async () => {
    const torrent = createTorrent([{ name: "movie.webm" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const error = vi.fn();
    service.on("error", error);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("error", new Error("tracker failed"));

    await expect(result).rejects.toThrow("tracker failed");
    expect(error).toHaveBeenCalledWith(expect.objectContaining({ message: "tracker failed" }));
  });

  it("emits discovered public peers when discovery reports peers", async () => {
    const torrent = createTorrent([{ name: "movie.webm" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const peerCount = vi.fn();

    service.on("peerCount", peerCount);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("metadata");
    await torrent.emit("peer", "192.0.2.10:6881");
    await torrent.emit("peer", "192.0.2.10:6881");

    await expect(result).resolves.toBe(torrent);
    expect(peerCount).toHaveBeenCalledWith(0);
    expect(peerCount).toHaveBeenCalledWith(1);
    expect(peerCount).not.toHaveBeenCalledWith(2);
  });

  it("streams selected files without creating the torrent client", async () => {
    const service = new TorrentService();
    const file = createTorrent([{ name: "movie.mkv" }]).files[0];
    const video = {
      removeAttribute: vi.fn(),
      load: vi.fn(),
      pause: vi.fn(),
    } as unknown as HTMLVideoElement;

    await service.streamToMedia(file, video);
    service.destroy();

    expect(video.pause).toHaveBeenCalledOnce();
    expect(video.removeAttribute).toHaveBeenCalledWith("src");
    expect(video.load).toHaveBeenCalledTimes(2);
    expect(file.streamTo).toHaveBeenCalledWith(video);
    expect(destroyMock).not.toHaveBeenCalled();
  });

  it("uses direct stream URLs when the backend provides them", async () => {
    const service = new TorrentService();
    const file = {
      name: "movie.mkv",
      length: 1024,
      streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie.mkv",
      streamTo: vi.fn(),
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const setSrc = vi.fn();
    const video = ({
      removeAttribute: vi.fn(),
      load: vi.fn(),
      pause: vi.fn(),
      addEventListener: vi.fn((_event: string, callback: () => void, _opts?: unknown) => {
        if (_event === "canplay") {
          callback();
        }
      }),
      removeEventListener: vi.fn(),
    } as unknown) as HTMLVideoElement & { src?: string };

    Object.defineProperty(video, "src", {
      configurable: true,
      set: setSrc,
    });

    await service.streamToMedia(file, video);

    expect(setSrc).toHaveBeenCalledWith("http://127.0.0.1:4321/webtorrent/hash/movie.mkv");
    expect(file.streamTo).not.toHaveBeenCalled();
    expect(video.load).toHaveBeenCalledTimes(2);
  });

  it("uses the Electron backend and keeps refreshed file stats aligned by source index", async () => {
    const initialSnapshot = {
      files: [
        {
          index: 0,
          name: "movie-a.mp4",
          length: 1_000,
          kind: "video",
          extension: ".mp4",
          progress: 0.1,
          streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie-a.mp4",
        },
        {
          index: 1,
          name: "movie-b.mp4",
          length: 2_000,
          kind: "video",
          extension: ".mp4",
          progress: 0.2,
          streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie-b.mp4",
        },
      ],
      progress: 0.1,
      downloadSpeed: 100,
      numPeers: 2,
      discoveredPeerCount: 7,
    };
    const refreshedSnapshot = {
      files: [
        {
          index: 1,
          name: "movie-b.mp4",
          length: 2_000,
          kind: "video",
          extension: ".mp4",
          progress: 0.75,
          streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie-b-updated.mp4",
        },
        {
          index: 0,
          name: "movie-a.mp4",
          length: 1_000,
          kind: "video",
          extension: ".mp4",
          progress: 0.5,
          streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie-a-updated.mp4",
        },
      ],
      progress: 0.5,
      downloadSpeed: 300,
      numPeers: 4,
      discoveredPeerCount: 11,
    };
    const backend = {
      addMagnet: vi.fn().mockResolvedValue(initialSnapshot),
      addTorrentFile: vi.fn(),
      getStats: vi.fn().mockResolvedValue(refreshedSnapshot),
      clear: vi.fn().mockResolvedValue(undefined),
    };

    vi.stubGlobal("window", {
      clearInterval,
      setInterval,
      torrsyncElectronTorrent: backend,
    });

    const service = new TorrentService();
    const progress = vi.fn();
    const peerCount = vi.fn();
    service.on("progress", progress);
    service.on("peerCount", peerCount);

    const torrent = await service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(backend.getStats).toHaveBeenCalled());

    expect(service.isElectronBackendEnabled()).toBe(true);
    expect(backend.addMagnet).toHaveBeenCalledWith("magnet:?xt=urn:btih:test");
    expect(progress).toHaveBeenCalledWith(0.5);
    expect(peerCount).toHaveBeenCalledWith(11);
    expect(torrent.files[0]).toEqual(
      expect.objectContaining({
        index: 0,
        name: "movie-a.mp4",
        progress: 0.5,
        streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie-a-updated.mp4",
      }),
    );
    expect(torrent.files[1]).toEqual(
      expect.objectContaining({
        index: 1,
        name: "movie-b.mp4",
        progress: 0.75,
        streamUrl: "http://127.0.0.1:4321/webtorrent/hash/movie-b-updated.mp4",
      }),
    );
    expect(torrent).toEqual(expect.objectContaining({ discoveredPeerCount: 11 }));

    await service.clearActiveTorrentForAdd();
    expect(backend.clear).toHaveBeenCalled();
  });

  it("falls through to streamTo when no stream URL and no service worker", async () => {
    const streamToMock = vi.fn(async () => {});
    const blobMock = vi.fn();

    const service = new TorrentService();
    (service as unknown as { client: unknown }).client = {
      createServer: vi.fn(),
    };
    const file = {
      name: "movie.mkv",
      length: 1024,
      streamTo: streamToMock,
      blob: blobMock,
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const video = {
      removeAttribute: vi.fn(),
      load: vi.fn(),
      pause: vi.fn(),
    } as unknown as HTMLMediaElement;

    await service.streamToMedia(file, video);

    expect(streamToMock).toHaveBeenCalledWith(video);
    expect(blobMock).not.toHaveBeenCalled();
  });

  it("accepts a torrent file payload", async () => {
    const torrent = createTorrent([{ name: "movie.webm" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();

    const result = service.addTorrentFile(new Uint8Array([1, 2, 3, 4]));
    await vi.waitFor(() =>
      expect(addMock).toHaveBeenCalledWith(expect.any(Uint8Array)),
    );
    await torrent.emit("metadata");

    await expect(result).resolves.toBe(torrent);
  });

  it("enables browser WebTorrent trackers for peer discovery", async () => {
    const torrent = createTorrent([{ name: "movie.webm" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("metadata");

    await expect(result).resolves.toBe(torrent);
  });

  describe("buffer settings", () => {
    it("persists and retrieves buffer settings", () => {
      const service = new TorrentService();
      const mockStorage = new Map<string, string>();
      vi.stubGlobal("localStorage", {
        getItem: (key: string) => mockStorage.get(key) ?? null,
        setItem: (key: string, value: string) => mockStorage.set(key, value),
      });

      service.setBufferSettings(100, 1000);

      const settings = service.getBufferSettings();
      expect(settings.bufferWindowMB).toBe(100);
      expect(settings.maxBufferMB).toBe(1000);
    });

    it("applies maxBufferBytes to limit buffer window", () => {
      const service = new TorrentService();
      const mockStorage = new Map<string, string>();
      vi.stubGlobal("localStorage", {
        getItem: (key: string) => mockStorage.get(key) ?? null,
        setItem: (key: string, value: string) => mockStorage.set(key, value),
      });

      service.setBufferSettings(50, 100);

      expect(service.getBufferSettings().maxBufferMB).toBe(100);
    });

    it("enforces buffer limit by pausing torrent when exceeded", () => {
      const service = new TorrentService();
      const mockStorage = new Map<string, string>();
      vi.stubGlobal("localStorage", {
        getItem: (key: string) => mockStorage.get(key) ?? null,
        setItem: (key: string, value: string) => mockStorage.set(key, value),
      });

      service.setBufferSettings(50, 10);

      const torrent = {
        pause: vi.fn(),
        resume: vi.fn(),
        paused: false,
        downloaded: 15 * 1024 * 1024,
      };

      (service as unknown as { activeTorrent: typeof torrent }).activeTorrent = torrent;
      (service as unknown as { maxBufferBytes: number }).maxBufferBytes = 10 * 1024 * 1024;

      (service as unknown as { enforceBufferLimit: () => void }).enforceBufferLimit();

      expect(torrent.pause).toHaveBeenCalled();
      expect(torrent.resume).not.toHaveBeenCalled();
    });

    it("resumes torrent when buffer drops below 90% of limit", () => {
      const service = new TorrentService();
      const mockStorage = new Map<string, string>();
      vi.stubGlobal("localStorage", {
        getItem: (key: string) => mockStorage.get(key) ?? null,
        setItem: (key: string, value: string) => mockStorage.set(key, value),
      });

      service.setBufferSettings(50, 10);

      const torrent = {
        pause: vi.fn(),
        resume: vi.fn(),
        paused: true,
        downloaded: 8 * 1024 * 1024,
      };

      (service as unknown as { activeTorrent: typeof torrent }).activeTorrent = torrent;
      (service as unknown as { maxBufferBytes: number }).maxBufferBytes = 10 * 1024 * 1024;

      (service as unknown as { enforceBufferLimit: () => void }).enforceBufferLimit();

      expect(torrent.resume).toHaveBeenCalled();
      expect(torrent.pause).not.toHaveBeenCalled();
    });
  });
});
