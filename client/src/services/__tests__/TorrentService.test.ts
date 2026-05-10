import { afterEach, describe, expect, it, vi } from "vitest";
import { TorrentService } from "../TorrentService";

type TorrentEvent = "download" | "ready" | "error";
type TorrentCallback = (error?: Error) => void | Promise<void>;

const { addMock, destroyMock } = vi.hoisted(() => ({
  addMock: vi.fn(),
  destroyMock: vi.fn(),
}));

vi.mock("webtorrent", () => ({
  default: vi.fn(() => ({
    add: addMock,
    destroy: destroyMock,
  })),
}));

function createTorrent(files: Array<{ name: string; streamTo?: (video: HTMLVideoElement) => Promise<void> }>) {
  const listeners = new Map<TorrentEvent, TorrentCallback>();
  const torrent = {
    files: files.map((file) => ({
      streamTo: vi.fn().mockResolvedValue(undefined),
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
    await torrent.emit("download");
    await torrent.emit("ready");

    await expect(result).resolves.toBe(torrent);
    expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test");
    expect(progress).toHaveBeenCalledWith(0.35);
    expect(speed).toHaveBeenCalledWith(2048);
    expect(ready).toHaveBeenCalledWith(torrent, torrent.files[1]);
  });

  it("rejects and emits an error when a ready torrent has no supported video file", async () => {
    const torrent = createTorrent([{ name: "archive.zip" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const error = vi.fn();
    service.on("error", error);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await torrent.emit("ready");

    await expect(result).rejects.toThrow("No supported video file found in torrent");
    expect(error).toHaveBeenCalledWith(
      expect.objectContaining({ message: "No supported video file found in torrent" }),
    );
  });

  it("normalizes torrent errors", async () => {
    const torrent = createTorrent([{ name: "movie.webm" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const error = vi.fn();
    service.on("error", error);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await torrent.emit("error", new Error("tracker failed"));

    await expect(result).rejects.toThrow("tracker failed");
    expect(error).toHaveBeenCalledWith(expect.objectContaining({ message: "tracker failed" }));
  });

  it("streams selected files and destroys the torrent client", async () => {
    const service = new TorrentService();
    const file = createTorrent([{ name: "movie.mkv" }]).files[0];
    const video = {
      removeAttribute: vi.fn(),
      load: vi.fn(),
    } as unknown as HTMLVideoElement;

    await service.streamToVideo(file, video);
    service.destroy();

    expect(video.removeAttribute).toHaveBeenCalledWith("src");
    expect(video.load).toHaveBeenCalledOnce();
    expect(file.streamTo).toHaveBeenCalledWith(video);
    expect(destroyMock).toHaveBeenCalledOnce();
  });
});
