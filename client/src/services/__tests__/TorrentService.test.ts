import { afterEach, describe, expect, it, vi } from "vitest";
import { TorrentService } from "../TorrentService";

type TorrentEvent = "download" | "ready" | "error";
type TorrentCallback = (error?: Error) => void | Promise<void>;

const { addMock, createServerMock, destroyMock } = vi.hoisted(() => ({
  addMock: vi.fn(),
  createServerMock: vi.fn(),
  destroyMock: vi.fn(),
}));

vi.mock("webtorrent", () => ({
  default: vi.fn(() => ({
    add: addMock,
    createServer: createServerMock,
    destroy: destroyMock,
  })),
}));

function createTorrent(
  files: Array<{ name: string; length?: number; streamTo?: (video: HTMLMediaElement) => Promise<void> }>,
) {
  const listeners = new Map<TorrentEvent, TorrentCallback>();
  const torrent = {
    files: files.map((file) => ({
      streamTo: vi.fn().mockResolvedValue(undefined),
      length: 1024,
      ...file,
    })),
    progress: 0.35,
    downloadSpeed: 2048,
    on: vi.fn((event: TorrentEvent, callback: TorrentCallback) => {
      listeners.set(event, callback);
    }),
    emit: async (event: TorrentEvent, error?: Error) => {
      await listeners.get(event)?.(error);
    },
  };

  return torrent;
}

describe("TorrentService", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
  });

  it("emits progress and resolves with the first supported video file when ready", async () => {
    const torrent = createTorrent([{ name: "notes.txt" }, { name: "Movie.MP4" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const progress = vi.fn();
    const speed = vi.fn();
    const ready = vi.fn();

    service.on("progress", progress);
    service.on("speed", speed);
    service.on("ready", ready);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("download");
    await torrent.emit("ready");

    await expect(result).resolves.toBe(torrent);
    expect(progress).toHaveBeenCalledWith(0.35);
    expect(speed).toHaveBeenCalledWith(2048);
    expect(ready).toHaveBeenCalledWith(
      torrent,
      expect.objectContaining({
        name: "Movie.MP4",
        kind: "video",
      }),
    );
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

  it("rejects and emits an error when a ready torrent has no supported media file", async () => {
    const torrent = createTorrent([{ name: "archive.zip" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const error = vi.fn();
    service.on("error", error);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("ready");

    await expect(result).rejects.toThrow("No supported video or audio file found in torrent");
    expect(error).toHaveBeenCalledWith(
      expect.objectContaining({ message: "No supported video or audio file found in torrent" }),
    );
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
    expect(video.load).toHaveBeenCalledOnce();
    expect(file.streamTo).toHaveBeenCalledWith(video);
    expect(destroyMock).not.toHaveBeenCalled();
  });

  it("waits for an activated service worker before creating the stream server", async () => {
    let serverCreated = false;
    const readyRegistration = {
      active: { state: "activated" },
    } as ServiceWorkerRegistration;
    const pendingRegistration = {
      active: { state: "installing" },
    } as ServiceWorkerRegistration;
    const serviceWorker = {
      register: vi.fn().mockResolvedValue(pendingRegistration),
      ready: Promise.resolve(readyRegistration),
    };
    vi.stubGlobal("navigator", { serviceWorker });

    createServerMock.mockImplementation((options: { controller: ServiceWorkerRegistration }) => {
      if (options.controller !== readyRegistration) {
        throw new Error("Worker isn't activated");
      }
      serverCreated = true;
      return {};
    });

    const service = new TorrentService();
    (service as unknown as { client: unknown }).client = {
      createServer: createServerMock,
    };
    const file = {
      name: "movie.mkv",
      length: 1024,
      streamTo: vi.fn(async () => {
        if (!serverCreated) {
          throw new Error("No server created");
        }
      }),
      blob: vi.fn(),
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const video = {
      removeAttribute: vi.fn(),
      load: vi.fn(),
      pause: vi.fn(),
    } as unknown as HTMLVideoElement;

    await service.streamToMedia(file, video);

    expect(serviceWorker.register).toHaveBeenCalledWith("webtorrent-sw.js");
    expect(createServerMock).toHaveBeenCalledWith({ controller: readyRegistration });
    expect(file.streamTo).toHaveBeenCalledWith(video);
    expect(file.blob).not.toHaveBeenCalled();
  });

  it("accepts a torrent file payload", async () => {
    const torrent = createTorrent([{ name: "movie.webm" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();

    const result = service.addTorrentFile(new Uint8Array([1, 2, 3, 4]));
    await vi.waitFor(() =>
      expect(addMock).toHaveBeenCalledWith(expect.any(Uint8Array)),
    );
    await torrent.emit("ready");

    await expect(result).resolves.toBe(torrent);
  });
});
