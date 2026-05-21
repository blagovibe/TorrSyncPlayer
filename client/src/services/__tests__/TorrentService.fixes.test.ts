import { afterEach, describe, expect, it, vi } from "vitest";
import { TorrentService } from "../TorrentService";

type TorrentEvent = "download" | "metadata" | "ready" | "error" | "wire" | "noPeers" | "peer";
type TorrentCallback = (...args: unknown[]) => void | Promise<void>;

const { addMock, createServerMock, destroyMock, webTorrentMock } = vi.hoisted(() => ({
  addMock: vi.fn(),
  createServerMock: vi.fn(),
  destroyMock: vi.fn(),
  webTorrentMock: vi.fn(() => ({
    add: addMock,
    createServer: createServerMock,
    destroy: destroyMock,
  })),
}));

vi.mock("webtorrent", () => ({
  default: webTorrentMock,
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
    numPeers: 0,
    on: vi.fn((event: string, callback: TorrentCallback) => {
      listeners.set(event as TorrentEvent, callback);
    }),
    emit: async (event: string, ...args: unknown[]) => {
      if (event === "wire") {
        torrent.numPeers += 1;
      }
      await listeners.get(event as TorrentEvent)?.(...args);
    },
  };

  return torrent;
}

describe("TorrentService fixes", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    if (typeof window !== "undefined") {
      delete (window as Window & { torrsyncElectronTorrent?: unknown }).torrsyncElectronTorrent;
    }
  });

  it("emits fewer progress events with deduplication than without", async () => {
    const torrent = createTorrent([{ name: "movie.mp4" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const progress = vi.fn();
    service.on("progress", progress);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("metadata");
    await expect(result).resolves.toBe(torrent);

    const callsAfterMetadata = progress.mock.calls.length;

    for (let i = 0; i < 10; i++) {
      await torrent.emit("download");
    }

    const totalCalls = progress.mock.calls.length;
    expect(totalCalls).toBeLessThan(callsAfterMetadata + 10);
  });

  it("emits progress when value actually changes", async () => {
    const torrent = createTorrent([{ name: "movie.mp4" }]);
    addMock.mockReturnValue(torrent);
    const service = new TorrentService();
    const progress = vi.fn();
    service.on("progress", progress);

    const result = service.addMagnet("magnet:?xt=urn:btih:test");
    await vi.waitFor(() => expect(addMock).toHaveBeenCalledWith("magnet:?xt=urn:btih:test"));
    await torrent.emit("metadata");
    await expect(result).resolves.toBe(torrent);

    const callsAfterMetadata = progress.mock.calls.length;

    torrent.progress = 0.55;
    await torrent.emit("download");

    expect(progress.mock.calls.length).toBe(callsAfterMetadata + 1);
    expect(progress).toHaveBeenCalledWith(0.55);
  });

  it("falls back to blob when streamTo fails with non-server error", async () => {
    const service = new TorrentService();
    const blobData = new Blob(["test"]);
    const file = {
      name: "movie.mkv",
      length: 1024,
      streamTo: vi.fn().mockRejectedValue(new Error("codec not supported")),
      blob: vi.fn().mockResolvedValue(blobData),
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const video = {
      pause: vi.fn(),
      removeAttribute: vi.fn(),
      load: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as HTMLMediaElement;

    const createObjectURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:test-url");

    await service.streamToMedia(file, video);

    expect(file.streamTo).toHaveBeenCalledWith(video);
    expect(file.blob).toHaveBeenCalled();
    expect(createObjectURL).toHaveBeenCalledWith(blobData);

    createObjectURL.mockRestore();
    await service.destroy();
  });

  it("resets playback position when switching files", async () => {
    const service = new TorrentService();
    const file1 = {
      name: "movie-a.mp4",
      length: 1024,
      streamTo: vi.fn().mockResolvedValue(undefined),
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const file2 = {
      name: "movie-b.mp4",
      length: 2048,
      streamTo: vi.fn().mockResolvedValue(undefined),
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const video = {
      pause: vi.fn(),
      removeAttribute: vi.fn(),
      load: vi.fn(),
    } as unknown as HTMLMediaElement;

    await service.streamToMedia(file1, video);

    service.updatePlaybackPosition(100, 1024, 1000);
    const bufferAfter = service.getBufferWindow();
    expect(bufferAfter.endMB).toBeGreaterThanOrEqual(0);

    await service.streamToMedia(file2, video);

    const bufferAfterSwitch = service.getBufferWindow();
    expect(bufferAfterSwitch.startMB).toBe(0);

    await service.destroy();
  });

  it("cleans up blob URLs on destroy", async () => {
    const service = new TorrentService();
    const blobData = new Blob(["test"]);
    const file = {
      name: "movie.mkv",
      length: 1024,
      streamTo: vi.fn().mockRejectedValue(new Error("fail")),
      blob: vi.fn().mockResolvedValue(blobData),
    } as unknown as Parameters<TorrentService["streamToMedia"]>[0];
    const video = {
      pause: vi.fn(),
      removeAttribute: vi.fn(),
      load: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as HTMLMediaElement;

    const revokeObjectURL = vi.spyOn(URL, "revokeObjectURL");
    const createObjectURL = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:test-url");

    await service.streamToMedia(file, video);
    await service.destroy();

    expect(revokeObjectURL).toHaveBeenCalledWith("blob:test-url");

    createObjectURL.mockRestore();
    revokeObjectURL.mockRestore();
  });
});
