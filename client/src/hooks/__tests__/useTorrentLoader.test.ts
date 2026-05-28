// @vitest-environment jsdom
import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useTorrentLoader } from "../useTorrentLoader";
import { RoomStateProvider } from "../../contexts/RoomStateContext";
import TorrentService, { type TorrentMediaFile } from "../../services/TorrentService";

// Mock TorrentService
const mockAddMagnet = vi.fn();
const mockAddTorrentFile = vi.fn();
const mockGetPlayableMediaFiles = vi.fn();
const mockGetPreferredMediaFile = vi.fn();
const mockStreamToMedia = vi.fn();
const mockProbeAudioTracks = vi.fn();
const mockProbeSubtitles = vi.fn();
const mockClearActiveTorrent = vi.fn();
const mockDestroy = vi.fn();
const mockIsDestroyed = vi.fn().mockReturnValue(false);
const mockIsDestroying = vi.fn().mockReturnValue(false);

vi.mock("../../services/TorrentService", () => {
  return {
    __esModule: true,
    default: vi.fn().mockImplementation(() => ({
      addMagnet: mockAddMagnet,
      addTorrentFile: mockAddTorrentFile,
      getPlayableMediaFiles: mockGetPlayableMediaFiles,
      getPreferredMediaFile: mockGetPreferredMediaFile,
      streamToMedia: mockStreamToMedia,
      probeAudioTracks: mockProbeAudioTracks,
      probeSubtitles: mockProbeSubtitles,
      clearActiveTorrent: mockClearActiveTorrent,
      destroy: mockDestroy,
      isDestroyed: mockIsDestroyed,
      isDestroying: mockIsDestroying,
      on: vi.fn().mockReturnValue(vi.fn()),
    })),
    type: {} as unknown,
  };
});

// Mock torrent utility
vi.mock("../../utils/torrent", () => ({
  createTorrentFileSource: vi.fn((name: string, bytes: Uint8Array) => ({
    kind: "file" as const,
    fileName: name,
    bytes,
    sourceKey: `file-${name}`,
  })),
}));

// Mock syncUtils
vi.mock("../../utils/syncUtils", () => ({
  isPlaybackBlockedError: vi.fn((error: unknown) => {
    return error instanceof Error && error.name === "NotAllowedError";
  }),
}));

// Mock logger
vi.mock("../../utils/logger", () => ({
  uiLogger: {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  },
}));

// Mock config
vi.mock("../../config", () => ({
  MAX_TORRENT_FILE_BYTES: 100 * 1024 * 1024, // 100 MB
}));

const createMockVideoElement = () => {
  return {
    play: vi.fn().mockResolvedValue(undefined),
    pause: vi.fn(),
    load: vi.fn(),
    removeAttribute: vi.fn(),
    setAttribute: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
    paused: true,
    muted: false,
    volume: 1,
    currentTime: 0,
    duration: 0,
    src: "",
    error: null,
    crossOrigin: "",
  } as unknown as HTMLVideoElement;
};

const createMockMediaFile = (overrides = {}): TorrentMediaFile => ({
  index: 0,
  name: "video.mp4",
  length: 1024000,
  kind: "video",
  extension: ".mp4",
  file: {
    name: "video.mp4",
    length: 1024000,
    streamTo: vi.fn().mockResolvedValue(undefined),
    streamUrl: "http://localhost/stream/0",
  },
  ...overrides,
});

const Wrapper = ({ children }: { children: React.ReactNode }) => {
  return React.createElement(RoomStateProvider, null, children);
};

describe("useTorrentLoader", () => {
  let mockVideoRef: React.RefObject<HTMLVideoElement | null>;
  let mockScheduleBroadcast: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockVideoRef = { current: createMockVideoElement() };
    mockScheduleBroadcast = vi.fn();

    // Default mock implementations
    mockAddMagnet.mockResolvedValue({
      infoHash: "abc123",
      files: [{ name: "video.mp4", length: 1024000 }],
    });
    mockAddTorrentFile.mockResolvedValue({
      infoHash: "def456",
      files: [{ name: "video.mp4", length: 1024000 }],
    });
    mockGetPlayableMediaFiles.mockReturnValue([createMockMediaFile()]);
    mockGetPreferredMediaFile.mockReturnValue(createMockMediaFile());
    mockStreamToMedia.mockResolvedValue(undefined);
    mockProbeAudioTracks.mockResolvedValue([]);
    mockProbeSubtitles.mockResolvedValue([]);
    mockClearActiveTorrent.mockResolvedValue(undefined);
    mockDestroy.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("Initial state", () => {
    it("returns initial state correctly", () => {
      const { result } = renderHook(
        () => useTorrentLoader(mockVideoRef, "home", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      expect(result.current.isLoadingTorrent).toBe(false);
      expect(result.current.torrentProgress).toBe(0);
      expect(result.current.downloadSpeed).toBe("0 B/s");
      expect(result.current.torrentError).toBeNull();
      expect(result.current.torrentPeerCount).toBe(0);
      expect(result.current.trackerLost).toBe(false);
      expect(result.current.playbackNotice).toBeNull();
      expect(result.current.mediaFiles).toEqual([]);
    });

    it("provides all required methods", () => {
      const { result } = renderHook(
        () => useTorrentLoader(mockVideoRef, "home", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      expect(typeof result.current.getTorrentService).toBe("function");
      expect(typeof result.current.loadTorrent).toBe("function");
      expect(typeof result.current.loadTorrentFile).toBe("function");
      expect(typeof result.current.resetTorrent).toBe("function");
    });
  });

  describe("getTorrentService", () => {
    it("returns TorrentService instance", () => {
      const { result } = renderHook(
        () => useTorrentLoader(mockVideoRef, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      const service = result.current.getTorrentService();

      expect(service).toBeDefined();
      expect(service.isDestroyed()).toBe(false);
    });

    it("creates new service if destroyed", () => {
      mockIsDestroyed.mockReturnValueOnce(true);

      const { result } = renderHook(
        () => useTorrentLoader(mockVideoRef, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      result.current.getTorrentService();

      // Should create new instance
      expect(TorrentService).toHaveBeenCalled();
    });

    it("creates new service if destroying", () => {
      mockIsDestroying.mockReturnValueOnce(true);

      const { result } = renderHook(
        () => useTorrentLoader(mockVideoRef, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      result.current.getTorrentService();

      // Should create new instance
      expect(TorrentService).toHaveBeenCalled();
    });
  });
});
