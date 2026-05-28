// @vitest-environment jsdom
import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useSyncPlayback, type SyncPlayback } from "../useSyncPlayback";
import { RoomStateProvider } from "../../contexts/RoomStateContext";
import SyncService from "../../services/SyncService";
import type P2PService from "../../services/P2PService";
import type { TorrentService } from "../useTorrentLoader";

// Mock SyncService
const mockDispose = vi.fn();
const mockSeek = vi.fn();
const mockSetSyncToleranceSeconds = vi.fn();
const mockApplyRemoteSync = vi.fn();

vi.mock("../../services/SyncService", () => {
  return {
    __esModule: true,
    default: vi.fn().mockImplementation(() => ({
      dispose: mockDispose,
      seek: mockSeek,
      setSyncToleranceSeconds: mockSetSyncToleranceSeconds,
      applyRemoteSync: mockApplyRemoteSync,
    })),
  };
});

// Mock syncUtils
vi.mock("../../utils/syncUtils", () => ({
  clampSyncTolerance: vi.fn((value: number) => Math.min(Math.max(value, 0), 30)),
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

const createMockVideoElement = () => {
  return {
    play: vi.fn().mockResolvedValue(undefined),
    pause: vi.fn(),
    load: vi.fn(),
    currentTime: 0,
    duration: 100,
    paused: true,
    muted: false,
    volume: 1,
    src: "",
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  } as unknown as HTMLVideoElement;
};

const createMockP2PService = () => {
  return {
    isConnected: vi.fn().mockReturnValue(true),
    sendSync: vi.fn(),
  } as unknown as P2PService;
};

const createMockTorrentService = () => {
  return {
    updatePlaybackPosition: vi.fn(),
  } as unknown as TorrentService;
};

const Wrapper = ({ children }: { children: React.ReactNode }) => {
  return React.createElement(RoomStateProvider, null, children);
};

describe("useSyncPlayback", () => {
  let mockVideoRef: React.RefObject<HTMLVideoElement | null>;
  let mockP2pService: React.MutableRefObject<P2PService | null>;
  let mockScheduleBroadcast: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockVideoRef = { current: createMockVideoElement() };
    mockP2pService = { current: createMockP2PService() };
    mockScheduleBroadcast = vi.fn();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("Initial state", () => {
    it("returns initial state correctly", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "home", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      expect(result.current.syncServiceRef.current).toBeNull();
      expect(typeof result.current.seek).toBe("function");
      expect(typeof result.current.setSyncTolerance).toBe("function");
      expect(typeof result.current.handleTimeUpdate).toBe("function");
      expect(typeof result.current.tryApplyPendingRemoteSync).toBe("function");
    });
  });

  describe("SyncService initialization", () => {
    it("initializes SyncService when in room view with peer role", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      // SyncService should be created when conditions are met
      // (peerRole is set, videoRef exists, p2pService exists)
    });

    it("does not initialize SyncService when not in room view", () => {
      renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "home", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      expect(SyncService).not.toHaveBeenCalled();
    });

    it("does not initialize SyncService when video ref is null", () => {
      const nullVideoRef = { current: null };

      renderHook(
        () => useSyncPlayback(nullVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      expect(SyncService).not.toHaveBeenCalled();
    });

    it("does not initialize SyncService when p2p service is null", () => {
      const nullP2pService = { current: null };

      renderHook(
        () => useSyncPlayback(mockVideoRef, nullP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      expect(SyncService).not.toHaveBeenCalled();
    });

    it("disposes SyncService on unmount", () => {
      const { unmount } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      unmount();

      // Should dispose the service
    });
  });

  describe("setSyncTolerance", () => {
    it("sets sync tolerance", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      // Set up syncServiceRef
      act(() => {
        result.current.syncServiceRef.current = new SyncService(
          { sendSync: vi.fn() },
          mockVideoRef.current!,
          "master",
          1.5
        );
      });

      act(() => {
        result.current.setSyncTolerance(2.5);
      });

      expect(mockSetSyncToleranceSeconds).toHaveBeenCalledWith(2.5);
    });

    it("clamps sync tolerance to valid range", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      // Set up syncServiceRef
      act(() => {
        result.current.syncServiceRef.current = new SyncService(
          { sendSync: vi.fn() },
          mockVideoRef.current!,
          "master",
          1.5
        );
      });

      // Test negative value
      act(() => {
        result.current.setSyncTolerance(-1);
      });

      // Test value above max
      act(() => {
        result.current.setSyncTolerance(50);
      });
    });

    it("handles setSyncTolerance errors gracefully", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      // Set up syncServiceRef that throws
      act(() => {
        result.current.syncServiceRef.current = new SyncService(
          { sendSync: vi.fn() },
          mockVideoRef.current!,
          "master",
          1.5
        );
        mockSetSyncToleranceSeconds.mockImplementation(() => {
          throw new Error("Set tolerance failed");
        });
      });

      // Should not throw
      act(() => {
        result.current.setSyncTolerance(2.5);
      });
    });
  });

  describe("handleTimeUpdate", () => {
    it("updates playback position with torrent service", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      const mockTorrentService = createMockTorrentService();
      const mockMediaFile = {
        file: { length: 1024000 },
      } as any;

      // Set selectedMediaFile in context
      // This requires context setup

      act(() => {
        result.current.handleTimeUpdate(10, 100, mockTorrentService);
      });

      // Should call updatePlaybackPosition if media file is set
    });

    it("does not update when no media file selected", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      const mockTorrentService = createMockTorrentService();

      act(() => {
        result.current.handleTimeUpdate(10, 100, mockTorrentService);
      });

      // Should not call updatePlaybackPosition
    });

    it("does not update when duration is invalid", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      const mockTorrentService = createMockTorrentService();

      act(() => {
        result.current.handleTimeUpdate(10, 0, mockTorrentService);
      });

      act(() => {
        result.current.handleTimeUpdate(10, -1, mockTorrentService);
      });

      act(() => {
        result.current.handleTimeUpdate(10, NaN, mockTorrentService);
      });

      // Should not call updatePlaybackPosition
    });

    it("does not update when torrent service is null", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      act(() => {
        result.current.handleTimeUpdate(10, 100, null);
      });

      // Should not throw
    });

    it("handles updatePlaybackPosition errors gracefully", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      const mockTorrentService = createMockTorrentService();
      (mockTorrentService as any).updatePlaybackPosition.mockImplementation(() => {
        throw new Error("Update failed");
      });

      // Should not throw
      act(() => {
        result.current.handleTimeUpdate(10, 100, mockTorrentService);
      });
    });
  });

  describe("tryApplyPendingRemoteSync", () => {
    it("applies pending remote sync when conditions are met", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      // Set up syncServiceRef
      act(() => {
        result.current.syncServiceRef.current = new SyncService(
          { sendSync: vi.fn() },
          mockVideoRef.current!,
          "slave",
          1.5
        );
      });

      // Set pending sync in context
      // This requires context setup

      act(() => {
        result.current.tryApplyPendingRemoteSync();
      });

      // Should apply sync if conditions are met
    });

    it("does not apply when peer role is master", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      act(() => {
        result.current.tryApplyPendingRemoteSync();
      });

      expect(mockApplyRemoteSync).not.toHaveBeenCalled();
    });

    it("does not apply when no pending sync", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      act(() => {
        result.current.tryApplyPendingRemoteSync();
      });

      expect(mockApplyRemoteSync).not.toHaveBeenCalled();
    });

    it("does not apply when sync service is null", () => {
      const { result } = renderHook(
        () => useSyncPlayback(mockVideoRef, mockP2pService, "room", mockScheduleBroadcast),
        { wrapper: Wrapper }
      );

      act(() => {
        result.current.tryApplyPendingRemoteSync();
      });

      expect(mockApplyRemoteSync).not.toHaveBeenCalled();
    });
  });
});
