// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import TorrentService from "../TorrentService";
import type { ElectronTorrentBackendAdapter } from "../torrent-backend";

// Mock the torrent-backend module
const mockAddMagnet = vi.fn();
const mockAddTorrentFile = vi.fn();
const mockGetStats = vi.fn();
const mockClear = vi.fn();
const mockSetMaxBufferMB = vi.fn();
const mockProbeAudioTracks = vi.fn();
const mockProbeSubtitles = vi.fn();
const mockCreateAudioTrackStreamUrl = vi.fn();
const mockCreateMultiplexedStreamUrl = vi.fn();
const mockCreateSubtitleStreamUrl = vi.fn();

const createMockElectronBackend = (): ElectronTorrentBackendAdapter => {
  return {
    addMagnet: mockAddMagnet,
    addTorrentFile: mockAddTorrentFile,
    getStats: mockGetStats,
    clear: mockClear,
    setMaxBufferMB: mockSetMaxBufferMB,
    probeAudioTracks: mockProbeAudioTracks,
    probeSubtitles: mockProbeSubtitles,
    createAudioTrackStreamUrl: mockCreateAudioTrackStreamUrl,
    createMultiplexedStreamUrl: mockCreateMultiplexedStreamUrl,
    createSubtitleStreamUrl: mockCreateSubtitleStreamUrl,
    destroy: vi.fn().mockResolvedValue(undefined),
  } as unknown as ElectronTorrentBackendAdapter;
};

const createMockTorrentInstance = (overrides = {}) => ({
  infoHash: "abc123",
  name: "Test Torrent",
  progress: 0.5,
  downloadSpeed: 1024,
  numPeers: 5,
  discoveredPeerCount: 5,
  files: [
    {
      index: 0,
      name: "video.mp4",
      length: 1024000,
      progress: 0.5,
      streamUrl: "http://localhost:12345/stream/0",
    },
    {
      index: 1,
      name: "subtitle.srt",
      length: 10240,
      progress: 1,
      streamUrl: "http://localhost:12345/stream/1",
    },
  ],
  ...overrides,
});

// Mock cleanup utility
vi.mock("../utils/cleanup", () => ({
  createCleanup: () => ({
    add: vi.fn(),
    setTimeout: vi.fn().mockReturnValue(1),
    setInterval: vi.fn().mockReturnValue(2),
    abort: vi.fn(),
  }),
  type: {} as unknown,
}));

describe("TorrentService Electron backend tests", () => {
  let originalWindow: typeof globalThis.window;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    originalWindow = globalThis.window;
  });

  afterEach(() => {
    vi.useRealTimers();
    // Clean up window mock
    globalThis.window = originalWindow;
  });

  const setupElectronBackend = () => {
    const backend = createMockElectronBackend();
    (globalThis.window as unknown as Record<string, unknown>).torrsyncElectronTorrent = backend;
    return backend;
  };

  const cleanupElectronBackend = () => {
    delete (globalThis.window as unknown as Record<string, unknown>).torrsyncElectronTorrent;
  };

  describe("Electron backend detection", () => {
    it("detects when Electron backend is available", () => {
      setupElectronBackend();
      const service = new TorrentService();

      expect(service.isElectronBackendEnabled()).toBe(true);

      cleanupElectronBackend();
    });

    it("detects when Electron backend is not available", () => {
      cleanupElectronBackend();
      const service = new TorrentService();

      expect(service.isElectronBackendEnabled()).toBe(false);
    });
  });

  describe("Adding torrents via Electron backend", () => {
    it("adds magnet link via Electron backend", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance();
      mockAddMagnet.mockResolvedValue(mockTorrent);

      const magnetLink = "magnet:?xt=urn:btih:abc123";
      const result = await service.addMagnet(magnetLink);

      expect(mockAddMagnet).toHaveBeenCalledWith(magnetLink);
      expect(result).toEqual(mockTorrent);

      cleanupElectronBackend();
    });

    it("adds torrent file via Electron backend", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance();
      mockAddTorrentFile.mockResolvedValue(mockTorrent);

      const torrentBytes = new Uint8Array([1, 2, 3, 4]);
      const result = await service.addTorrentFile(torrentBytes);

      expect(mockAddTorrentFile).toHaveBeenCalledWith(torrentBytes);
      expect(result).toEqual(mockTorrent);

      cleanupElectronBackend();
    });

    it("emits error when Electron backend fails to add magnet", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const error = new Error("Failed to add magnet");
      mockAddMagnet.mockRejectedValue(error);

      const errorHandler = vi.fn();
      service.on("error", errorHandler);

      const magnetLink = "magnet:?xt=urn:btih:invalid";
      await expect(service.addMagnet(magnetLink)).rejects.toThrow("Failed to add magnet");

      expect(errorHandler).toHaveBeenCalledWith(error);

      cleanupElectronBackend();
    });

    it("emits error when Electron backend fails to add torrent file", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const error = new Error("Failed to add torrent file");
      mockAddTorrentFile.mockRejectedValue(error);

      const errorHandler = vi.fn();
      service.on("error", errorHandler);

      const torrentBytes = new Uint8Array([1, 2, 3, 4]);
      await expect(service.addTorrentFile(torrentBytes)).rejects.toThrow("Failed to add torrent file");

      expect(errorHandler).toHaveBeenCalledWith(error);

      cleanupElectronBackend();
    });
  });

  describe("Buffer settings", () => {
    it("sets buffer settings on Electron backend", () => {
      setupElectronBackend();
      const service = new TorrentService();

      service.setBufferSettings(100, 1000);

      expect(mockSetMaxBufferMB).toHaveBeenCalledWith(1000);

      cleanupElectronBackend();
    });

    it("gets current buffer settings", () => {
      const service = new TorrentService();

      const settings = service.getBufferSettings();

      expect(settings).toHaveProperty("bufferWindowMB");
      expect(settings).toHaveProperty("maxBufferMB");
    });
  });

  describe("Media file operations", () => {
    it("gets playable media files from torrent", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance();
      mockAddMagnet.mockResolvedValue(mockTorrent);

      await service.addMagnet("magnet:?xt=urn:btih:abc123");

      const mediaFiles = service.getPlayableMediaFiles(mockTorrent);

      expect(mediaFiles.length).toBeGreaterThan(0);
      expect(mediaFiles[0].name).toBe("video.mp4");

      cleanupElectronBackend();
    });

    it("gets preferred media file from torrent", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance();
      mockAddMagnet.mockResolvedValue(mockTorrent);

      await service.addMagnet("magnet:?xt=urn:btih:abc123");

      const preferredFile = service.getPreferredMediaFile(mockTorrent);

      expect(preferredFile.name).toBe("video.mp4");

      cleanupElectronBackend();
    });

    it("throws error when no supported media files found", () => {
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance({
        files: [
          { index: 0, name: "readme.txt", length: 1024, progress: 1 },
        ],
      });

      expect(() => service.getPreferredMediaFile(mockTorrent)).toThrow(
        "No supported video or audio file found"
      );
    });
  });

  describe("Audio track operations", () => {
    it("probes audio tracks via Electron backend", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTracks = [
        { index: 0, label: "English", language: "en", codecName: "aac", channels: 2, sampleRate: 48000 },
        { index: 1, label: "Spanish", language: "es", codecName: "aac", channels: 2, sampleRate: 48000 },
      ];
      mockProbeAudioTracks.mockResolvedValue(mockTracks);

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const tracks = await service.probeAudioTracks(file);

      expect(mockProbeAudioTracks).toHaveBeenCalledWith("http://localhost:12345/stream/0");
      expect(tracks).toEqual(mockTracks);

      cleanupElectronBackend();
    });

    it("returns empty array when audio probe fails", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      mockProbeAudioTracks.mockRejectedValue(new Error("Probe failed"));

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const tracks = await service.probeAudioTracks(file);

      expect(tracks).toEqual([]);

      cleanupElectronBackend();
    });

    it("returns empty array when no stream URL", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const file = { streamUrl: undefined } as any;
      const tracks = await service.probeAudioTracks(file);

      expect(tracks).toEqual([]);
      expect(mockProbeAudioTracks).not.toHaveBeenCalled();

      cleanupElectronBackend();
    });

    it("creates audio track stream URL", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const streamUrl = "http://localhost:12345/stream/0/audio/1";
      mockCreateAudioTrackStreamUrl.mockResolvedValue(streamUrl);

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const result = await service.createAudioTrackStreamUrl(file, 1, 0);

      expect(mockCreateAudioTrackStreamUrl).toHaveBeenCalledWith({
        streamUrl: "http://localhost:12345/stream/0",
        trackIndex: 1,
        startSeconds: 0,
      });
      expect(result).toBe(streamUrl);

      cleanupElectronBackend();
    });
  });

  describe("Subtitle operations", () => {
    it("probes subtitles via Electron backend", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockSubtitles = [
        { index: 0, label: "English", language: "en", codecName: "subrip", forced: false, default: true },
      ];
      mockProbeSubtitles.mockResolvedValue(mockSubtitles);

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const subtitles = await service.probeSubtitles(file);

      expect(mockProbeSubtitles).toHaveBeenCalledWith("http://localhost:12345/stream/0");
      expect(subtitles).toEqual(mockSubtitles);

      cleanupElectronBackend();
    });

    it("returns empty array when subtitle probe fails", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      mockProbeSubtitles.mockRejectedValue(new Error("Probe failed"));

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const subtitles = await service.probeSubtitles(file);

      expect(subtitles).toEqual([]);

      cleanupElectronBackend();
    });

    it("creates subtitle stream URL", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const streamUrl = "http://localhost:12345/stream/0/subtitle/0";
      mockCreateSubtitleStreamUrl.mockResolvedValue(streamUrl);

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const result = await service.createSubtitleStreamUrl(file, 0, 0);

      expect(mockCreateSubtitleStreamUrl).toHaveBeenCalledWith({
        streamUrl: "http://localhost:12345/stream/0",
        trackIndex: 0,
        startSeconds: 0,
      });
      expect(result).toBe(streamUrl);

      cleanupElectronBackend();
    });
  });

  describe("Mux stream operations", () => {
    it("creates multiplexed stream URL", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const muxUrl = "http://localhost:12345/stream/0/mux";
      mockCreateMultiplexedStreamUrl.mockResolvedValue(muxUrl);

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const result = await service.createMuxStreamUrl(file, 0, 0);

      expect(mockCreateMultiplexedStreamUrl).toHaveBeenCalledWith({
        streamUrl: "http://localhost:12345/stream/0",
        audioTrackIndex: 0,
        startSeconds: 0,
      });
      expect(result).toBe(muxUrl);

      cleanupElectronBackend();
    });

    it("returns null when mux creation fails", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      mockCreateMultiplexedStreamUrl.mockRejectedValue(new Error("Mux failed"));

      const file = { streamUrl: "http://localhost:12345/stream/0" } as any;
      const result = await service.createMuxStreamUrl(file, 0, 0);

      expect(result).toBeNull();

      cleanupElectronBackend();
    });
  });

  describe("Clear and destroy operations", () => {
    it("clears active torrent via Electron backend", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance();
      mockAddMagnet.mockResolvedValue(mockTorrent);

      await service.addMagnet("magnet:?xt=urn:btih:abc123");
      await service.clearActiveTorrent();

      expect(mockClear).toHaveBeenCalled();

      cleanupElectronBackend();
    });

    it("destroys service and clears Electron backend", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance();
      mockAddMagnet.mockResolvedValue(mockTorrent);

      await service.addMagnet("magnet:?xt=urn:btih:abc123");
      await service.destroy();

      expect(mockClear).toHaveBeenCalled();
      expect(service.isDestroyed()).toBe(true);

      cleanupElectronBackend();
    });

    it("prevents operations after destroy", async () => {
      setupElectronBackend();
      const service = new TorrentService();

      await service.destroy();

      await expect(service.addMagnet("magnet:?xt=urn:btih:abc123")).rejects.toThrow(
        "TorrentService has been destroyed"
      );

      cleanupElectronBackend();
    });
  });

  describe("Edge cases", () => {
    it("handles file with too many files", () => {
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance({
        files: Array.from({ length: 10001 }, (_, i) => ({
          index: i,
          name: `file${i}.mp4`,
          length: 1024,
          progress: 1,
        })),
      });

      expect(() => service.getPlayableMediaFiles(mockTorrent)).toThrow(
        "Torrent contains too many files"
      );
    });

    it("handles file with name too long", () => {
      const service = new TorrentService();

      const mockTorrent = createMockTorrentInstance({
        files: [
          { index: 0, name: "a".repeat(513), length: 1024, progress: 1 },
        ],
      });

      expect(() => service.getPlayableMediaFiles(mockTorrent)).toThrow(
        "Torrent file name exceeds maximum length"
      );
    });
  });
});
