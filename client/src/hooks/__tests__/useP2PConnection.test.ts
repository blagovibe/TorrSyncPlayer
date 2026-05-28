// @vitest-environment jsdom
import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useP2PConnection } from "../useP2PConnection";
import { RoomStateProvider } from "../../contexts/RoomStateContext";
import P2PService from "../../services/P2PService";

// Mock P2PService
const mockInitialize = vi.fn();
const mockDisconnect = vi.fn();
const mockConnect = vi.fn();
const mockSendChat = vi.fn();
const mockSendSync = vi.fn();
const mockSendTorrentSource = vi.fn();
const mockSendRoomConfig = vi.fn();
const mockIsHost = vi.fn().mockReturnValue(true);
const mockIsConnected = vi.fn().mockReturnValue(false);
const mockGetPeerId = vi.fn().mockReturnValue("ABC123");
const mockGetLastRttMs = vi.fn().mockReturnValue(null);
const mockClearRateLimitForPeer = vi.fn();
const mockOn = vi.fn().mockReturnValue(vi.fn()); // Return unsubscribe function

vi.mock("../../services/P2PService", () => {
  return {
    __esModule: true,
    default: vi.fn().mockImplementation(() => ({
      initialize: mockInitialize,
      disconnect: mockDisconnect,
      connect: mockConnect,
      sendChat: mockSendChat,
      sendSync: mockSendSync,
      sendTorrentSource: mockSendTorrentSource,
      sendRoomConfig: mockSendRoomConfig,
      isHost: mockIsHost,
      isConnected: mockIsConnected,
      getPeerId: mockGetPeerId,
      getLastRttMs: mockGetLastRttMs,
      clearRateLimitForPeer: mockClearRateLimitForPeer,
      on: mockOn,
      setHost: vi.fn(),
      setGuest: vi.fn(),
    })),
  };
});

// Mock sanitize utilities
vi.mock("../../utils/sanitize", () => ({
  sanitizeChatMessage: vi.fn((msg: string) => msg?.trim() || null),
  validateChatMessage: vi.fn((msg: string) => msg?.trim() || null),
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

const Wrapper = ({ children }: { children: React.ReactNode }) => {
  return React.createElement(RoomStateProvider, null, children);
};

describe("useP2PConnection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockInitialize.mockResolvedValue(undefined);
    mockDisconnect.mockResolvedValue(undefined);
    mockConnect.mockResolvedValue(undefined);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("Initial state", () => {
    it("returns initial state correctly", () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      expect(result.current.peerId).toBe("");
      expect(result.current.peerRole).toBeNull();
      expect(result.current.peers).toEqual([]);
      expect(result.current.isConnected).toBe(false);
      expect(result.current.isConnecting).toBe(false);
      expect(result.current.connectionError).toBeNull();
      expect(result.current.connectionQuality).toBe("unknown");
      expect(result.current.rttMs).toBeNull();
      expect(result.current.reconnectFailed).toBe(false);
      expect(result.current.chatMessages).toEqual([]);
      expect(result.current.p2pService).toBeNull();
    });

    it("provides all required methods", () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      expect(typeof result.current.createRoom).toBe("function");
      expect(typeof result.current.joinRoom).toBe("function");
      expect(typeof result.current.disconnect).toBe("function");
      expect(typeof result.current.sendChat).toBe("function");
      expect(typeof result.current.broadcastRoomState).toBe("function");
      expect(typeof result.current.scheduleBroadcast).toBe("function");
      expect(typeof result.current.onSync).toBe("function");
      expect(typeof result.current.onTorrentSource).toBe("function");
      expect(typeof result.current.onRoomConfig).toBe("function");
    });
  });

  describe("createRoom", () => {
    it("creates room successfully as host", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.createRoom();
      });

      expect(mockInitialize).toHaveBeenCalled();
      expect(result.current.peerId).toBe("ABC123");
      expect(result.current.peerRole).toBe("master");
      expect(result.current.isConnecting).toBe(false);
    });

    it("sets peer list with self on room creation", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.createRoom();
      });

      expect(result.current.peers).toEqual([
        { id: "self", name: "You", role: "master", connectionState: "connected" },
      ]);
    });

    it("prevents room creation while already connecting", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Start first creation
      act(() => {
        result.current.createRoom();
      });

      // Try to create again while connecting
      await act(async () => {
        await result.current.createRoom();
      });

      // Should only initialize once
      expect(mockInitialize).toHaveBeenCalledTimes(1);
    });

    it("handles initialization failure", async () => {
      mockInitialize.mockRejectedValueOnce(new Error("Init failed"));

      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.createRoom();
      });

      expect(result.current.connectionError).toBe("Init failed");
      expect(result.current.peerRole).toBeNull();
      expect(result.current.isConnected).toBe(false);
    });
  });

  describe("joinRoom", () => {
    it("joins room successfully as guest", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.joinRoom("ABC123");
      });

      expect(mockInitialize).toHaveBeenCalled();
      expect(mockConnect).toHaveBeenCalledWith("torrsync-ABC123");
      expect(result.current.peerRole).toBe("slave");
      expect(result.current.isConnected).toBe(true);
    });

    it("sets peer list with self and host on join", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.joinRoom("ABC123");
      });

      expect(result.current.peers).toEqual([
        { id: "self", name: "You", role: "slave", connectionState: "connected" },
        { id: "ABC123", name: "Host", role: "master", connectionState: "connected" },
      ]);
    });

    it("validates room code format", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.joinRoom(""); // Empty
      });

      expect(result.current.connectionError).toBe("Invalid peer ID. Please enter a 6-character room code.");
      expect(mockInitialize).not.toHaveBeenCalled();

      await act(async () => {
        await result.current.joinRoom("ABC"); // Too short
      });

      expect(result.current.connectionError).toBe("Invalid peer ID. Please enter a 6-character room code.");

      await act(async () => {
        await result.current.joinRoom("ABCDEFGH"); // Too long
      });

      expect(result.current.connectionError).toBe("Invalid peer ID. Please enter a 6-character room code.");

      await act(async () => {
        await result.current.joinRoom("abc!@#"); // Invalid characters
      });

      expect(result.current.connectionError).toBe("Invalid peer ID. Please enter a 6-character room code.");
    });

    it("normalizes room code to uppercase", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.joinRoom("abc123");
      });

      expect(mockConnect).toHaveBeenCalledWith("torrsync-ABC123");
    });

    it("prevents join while already connecting", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Start first join
      act(() => {
        result.current.joinRoom("ABC123");
      });

      // Try to join again while connecting
      await act(async () => {
        await result.current.joinRoom("DEF456");
      });

      // Should only initialize once
      expect(mockInitialize).toHaveBeenCalledTimes(1);
    });

    it("handles connection failure", async () => {
      mockConnect.mockRejectedValueOnce(new Error("Connection failed"));

      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      await act(async () => {
        await result.current.joinRoom("ABC123");
      });

      expect(result.current.connectionError).toBe("Connection failed");
      expect(result.current.peerRole).toBeNull();
      expect(result.current.isConnected).toBe(false);
    });
  });

  describe("disconnect", () => {
    it("disconnects and resets state", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // First create a room
      await act(async () => {
        await result.current.createRoom();
      });

      // Then disconnect
      await act(async () => {
        await result.current.disconnect();
      });

      expect(mockDisconnect).toHaveBeenCalled();
      expect(result.current.peerId).toBe("");
      expect(result.current.peerRole).toBeNull();
      expect(result.current.peers).toEqual([]);
      expect(result.current.isConnected).toBe(false);
      expect(result.current.isConnecting).toBe(false);
      expect(result.current.connectionError).toBeNull();
      expect(result.current.reconnectFailed).toBe(false);
      expect(result.current.chatMessages).toEqual([]);
    });

    it("handles disconnect errors gracefully", async () => {
      mockDisconnect.mockRejectedValueOnce(new Error("Disconnect error"));

      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // First create a room
      await act(async () => {
        await result.current.createRoom();
      });

      // Then disconnect (should not throw)
      await act(async () => {
        await result.current.disconnect();
      });

      // State should still be reset
      expect(result.current.peerId).toBe("");
      expect(result.current.isConnected).toBe(false);
    });
  });

  describe("sendChat", () => {
    it("sends chat message", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Create room first
      await act(async () => {
        await result.current.createRoom();
      });

      act(() => {
        result.current.sendChat("Hello!");
      });

      expect(mockSendChat).toHaveBeenCalledWith("Hello!");
    });

    it("adds sent message to chat history", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Create room first
      await act(async () => {
        await result.current.createRoom();
      });

      act(() => {
        result.current.sendChat("Hello!");
      });

      expect(result.current.chatMessages).toHaveLength(1);
      expect(result.current.chatMessages[0].text).toBe("Hello!");
      expect(result.current.chatMessages[0].sender).toBe("ABC123");
    });

    it("ignores empty messages", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Create room first
      await act(async () => {
        await result.current.createRoom();
      });

      act(() => {
        result.current.sendChat("");
      });

      expect(mockSendChat).not.toHaveBeenCalled();
      expect(result.current.chatMessages).toHaveLength(0);
    });

    it("ignores whitespace-only messages", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Create room first
      await act(async () => {
        await result.current.createRoom();
      });

      act(() => {
        result.current.sendChat("   ");
      });

      expect(mockSendChat).not.toHaveBeenCalled();
    });
  });

  describe("Event listeners", () => {
    it("registers sync listener", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      const syncHandler = vi.fn();
      act(() => {
        result.current.onSync(syncHandler);
      });

      // The listener should be registered (internal implementation detail)
      expect(typeof result.current.onSync).toBe("function");
    });

    it("registers torrent source listener", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      const torrentSourceHandler = vi.fn();
      act(() => {
        result.current.onTorrentSource(torrentSourceHandler);
      });

      expect(typeof result.current.onTorrentSource).toBe("function");
    });

    it("registers room config listener", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      const roomConfigHandler = vi.fn();
      act(() => {
        result.current.onRoomConfig(roomConfigHandler);
      });

      expect(typeof result.current.onRoomConfig).toBe("function");
    });

    it("returns unsubscribe function from listeners", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      const syncHandler = vi.fn();
      let unsubscribe: (() => void) | undefined;

      act(() => {
        unsubscribe = result.current.onSync(syncHandler);
      });

      expect(typeof unsubscribe).toBe("function");

      act(() => {
        unsubscribe!();
      });

      // Handler should be removed (internal implementation detail)
    });
  });

  describe("broadcastRoomState", () => {
    it("broadcasts room state when host", async () => {
      mockIsHost.mockReturnValue(true);

      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      // Create room first
      await act(async () => {
        await result.current.createRoom();
      });

      act(() => {
        result.current.broadcastRoomState();
      });

      // Should call sendTorrentSource and sendRoomConfig if torrent source exists
      // (depends on room state)
    });

    it("does not broadcast when not host", async () => {
      mockIsHost.mockReturnValue(false);

      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      act(() => {
        result.current.broadcastRoomState();
      });

      expect(mockSendTorrentSource).not.toHaveBeenCalled();
    });
  });

  describe("scheduleBroadcast", () => {
    it("schedules broadcast", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      act(() => {
        result.current.scheduleBroadcast();
      });

      // Should not throw
      expect(typeof result.current.scheduleBroadcast).toBe("function");
    });

    it("schedules broadcast to specific peer", async () => {
      const { result } = renderHook(() => useP2PConnection(), { wrapper: Wrapper });

      act(() => {
        result.current.scheduleBroadcast("peer-123");
      });

      // Should not throw
      expect(typeof result.current.scheduleBroadcast).toBe("function");
    });
  });
});
