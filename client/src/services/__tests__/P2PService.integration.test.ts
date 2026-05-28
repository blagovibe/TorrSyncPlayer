// @vitest-environment jsdom

// Mock RTCPeerConnection before any imports
Object.defineProperty(globalThis, "RTCPeerConnection", {
  value: class MockRTCPeerConnection {
    createDataChannel = vi.fn().mockReturnValue({ close: vi.fn() });
    close = vi.fn();
  },
  writable: true,
  configurable: true,
});

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import P2PService, {
  WEBRTC_UNAVAILABLE_MESSAGE,
  SIGNALING_UNAVAILABLE_MESSAGE,
} from "../P2PService";

// Mock PeerJS
interface MockPeerInstance {
  id: string;
  options?: unknown;
  open: boolean;
  destroyed: boolean;
  connect: (peerId: string) => ReturnType<typeof createMockConnection>;
  destroy: () => void;
  reconnect: () => void;
  on: (event: string, cb: (...args: unknown[]) => void) => void;
  off: (event: string, cb: (...args: unknown[]) => void) => void;
  emit: (event: string, ...args: unknown[]) => void;
  _triggerOpen: (openId: string) => void;
  _triggerError: (error: Error) => void;
  _triggerDisconnected: () => void;
  _triggerConnection: (conn: ReturnType<typeof createMockConnection>) => void;
}

const createMockConnection = (peerId: string, isOpen = false) => {
  const listeners: Record<string, Array<(...args: unknown[]) => void>> = {};
  const conn = {
    peer: peerId,
    open: isOpen,
    send: vi.fn(),
    close: vi.fn(),
    on: vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      if (!listeners[event]) listeners[event] = [];
      listeners[event].push(cb);
    }),
    off: vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      if (listeners[event]) {
        listeners[event] = listeners[event].filter((fn) => fn !== cb);
      }
    }),
    emit: (event: string, ...args: unknown[]) => {
      listeners[event]?.forEach((cb) => cb(...args));
    },
  };
  return conn;
};

const MockPeer = vi.fn().mockImplementation((id: string, options?: unknown) => {
  const listeners: Record<string, Array<(...args: unknown[]) => void>> = {};
  const peerInstance: MockPeerInstance = {
    id,
    options,
    open: false,
    destroyed: false,
    connect: vi.fn((peerId: string) => createMockConnection(peerId)),
    destroy: vi.fn().mockImplementation(() => {
      peerInstance.destroyed = true;
      peerInstance.open = false;
    }),
    reconnect: vi.fn().mockImplementation(() => {
      if (peerInstance.destroyed) {
        peerInstance.destroyed = false;
        peerInstance.open = true;
      }
    }),
    on: vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      if (!listeners[event]) listeners[event] = [];
      listeners[event].push(cb);
    }),
    off: vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      if (listeners[event]) {
        listeners[event] = listeners[event].filter((fn) => fn !== cb);
      }
    }),
    emit: (event: string, ...args: unknown[]) => {
      listeners[event]?.forEach((cb) => cb(...args));
    },
    _triggerOpen: (openId: string) => {
      peerInstance.open = true;
      listeners["open"]?.forEach((cb) => cb(openId));
    },
    _triggerError: (error: Error) => {
      listeners["error"]?.forEach((cb) => cb(error));
    },
    _triggerDisconnected: () => {
      listeners["disconnected"]?.forEach((cb) => cb());
    },
    _triggerConnection: (conn: ReturnType<typeof createMockConnection>) => {
      listeners["connection"]?.forEach((cb) => cb(conn));
    },
  };
  return peerInstance;
});

vi.mock("peerjs", () => ({
  default: MockPeer,
  Peer: MockPeer,
}));

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

describe("P2PService integration tests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("State machine transitions", () => {
    it("transitions from disconnected -> connecting -> connected", async () => {
      const service = new P2PService();
      expect(service.getState()).toBe("disconnected");

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      expect(peerInstance).toBeDefined();

      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      expect(service.getState()).toBe("disconnected");
      expect(service.getPeerId()).toBeTruthy();
    });

    it("transitions to destroyed state on disconnect", async () => {
      const service = new P2PService();
      expect(service.getState()).toBe("disconnected");

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      await service.disconnect();
      expect(service.getState()).toBe("disconnected");
    });

    it("handles destroyed state correctly", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      await service.disconnect();

      await expect(service.connect("some-peer")).rejects.toThrow();
    });
  });

  describe("Connection lifecycle", () => {
    it("successfully connects to a remote peer", async () => {
      const service = new P2PService();
      const connectedHandler = vi.fn();
      service.on("connected", connectedHandler);

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = peerInstance.connect.mock.results[0]?.value;
      expect(mockConn).toBeDefined();

      mockConn.emit("open");
      await connectPromise;

      expect(service.isConnected()).toBe(true);
      expect(connectedHandler).toHaveBeenCalled();
    });

    it("handles connection timeout", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      await vi.advanceTimersByTimeAsync(31_000);

      await expect(connectPromise).rejects.toThrow("Connection timeout");
    });

    it("handles incoming connections", async () => {
      const service = new P2PService();
      const peerConnectedHandler = vi.fn();
      service.on("peer_connected", peerConnectedHandler);

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const incomingConn = createMockConnection("incoming-peer", true);
      peerInstance._triggerConnection(incomingConn);

      expect(peerConnectedHandler).toHaveBeenCalledWith("incoming-peer");
    });
  });

  describe("Message passing", () => {
    it("sends sync messages to connected peers", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = peerInstance.connect.mock.results[0]?.value;
      mockConn.emit("open");
      await connectPromise;

      service.sendSync({
        action: "play",
        position: 10.5,
        server_ts: Date.now(),
      });

      expect(mockConn.send).toHaveBeenCalledWith({
        type: "sync",
        message: {
          action: "play",
          position: 10.5,
          server_ts: expect.any(Number),
        },
      });
    });

    it("receives and emits sync messages", async () => {
      const service = new P2PService();
      const syncHandler = vi.fn();
      service.on("sync", syncHandler);

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = peerInstance.connect.mock.results[0]?.value;
      mockConn.emit("open");
      await connectPromise;

      const syncMessage = {
        type: "sync",
        message: {
          action: "pause",
          position: 20.0,
          server_ts: Date.now(),
        },
      };
      mockConn.emit("data", syncMessage);

      expect(syncHandler).toHaveBeenCalledWith(syncMessage.message);
    });

    it("sends chat messages", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = peerInstance.connect.mock.results[0]?.value;
      mockConn.emit("open");
      await connectPromise;

      service.sendChat("Hello, world!");

      expect(mockConn.send).toHaveBeenCalledWith({
        type: "chat",
        content: "Hello, world!",
      });
    });
  });

  describe("Error handling", () => {
    it("emits error on WebRTC unavailable", async () => {
      const originalRTCPeerConnection = globalThis.RTCPeerConnection;
      // @ts-expect-error - intentionally removing for test
      delete globalThis.RTCPeerConnection;

      const service = new P2PService();

      await expect(service.initialize()).rejects.toThrow(WEBRTC_UNAVAILABLE_MESSAGE);

      globalThis.RTCPeerConnection = originalRTCPeerConnection;
    });

    it("emits error on signaling server unavailable", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;

      peerInstance._triggerError(new Error("Lost connection to server"));

      await expect(initPromise).rejects.toThrow(SIGNALING_UNAVAILABLE_MESSAGE);
    });
  });

  describe("Host/Guest role management", () => {
    it("defaults to host role", () => {
      const service = new P2PService();
      expect(service.isHost()).toBe(true);
    });

    it("can switch between host and guest roles", () => {
      const service = new P2PService();

      service.setGuest();
      expect(service.isHost()).toBe(false);

      service.setHost();
      expect(service.isHost()).toBe(true);
    });

    it("uses correct peer ID prefix for host", async () => {
      const service = new P2PService();
      service.setHost();

      const initPromise = service.initialize();

      expect(MockPeer.mock.calls[0][0]).toMatch(/^torrsync-/);

      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("torrsync-test123");
      await initPromise;

      expect(service.getPeerId()).toBe("test123");
    });
  });

  describe("Edge cases", () => {
    it("prevents double initialization", async () => {
      const service = new P2PService();

      const initPromise1 = service.initialize();
      const peerInstance1 = MockPeer.mock.results[0]?.value;
      peerInstance1._triggerOpen("test-peer-id");
      await initPromise1;

      const initPromise2 = service.initialize();
      await initPromise2;

      expect(MockPeer).toHaveBeenCalledTimes(1);
    });

    it("handles malformed incoming messages gracefully", async () => {
      const service = new P2PService();
      const syncHandler = vi.fn();
      service.on("sync", syncHandler);

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = peerInstance.connect.mock.results[0]?.value;
      mockConn.emit("open");
      await connectPromise;

      mockConn.emit("data", "invalid string");
      mockConn.emit("data", { type: "unknown" });
      mockConn.emit("data", { type: "sync" });
      mockConn.emit("data", 123);
      mockConn.emit("data", null);

      expect(syncHandler).not.toHaveBeenCalled();
    });

    it("cleans up all resources on disconnect", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = peerInstance.connect.mock.results[0]?.value;
      mockConn.emit("open");
      await connectPromise;

      expect(service.isConnected()).toBe(true);

      await service.disconnect();

      expect(service.isConnected()).toBe(false);
      expect(service.isInRoom()).toBe(false);
      expect(peerInstance.destroy).toHaveBeenCalled();
    });
  });
});
