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

import { describe, it, expect, vi, beforeEach } from "vitest";
import P2PService, {
  WEBRTC_UNAVAILABLE_MESSAGE,
  SIGNALING_UNAVAILABLE_MESSAGE,
} from "../P2PService";

// Mock PeerJS
interface MockConnection {
  peer: string;
  open: boolean;
  send: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
  on: ReturnType<typeof vi.fn>;
  off: ReturnType<typeof vi.fn>;
  emit: (event: string, ...args: unknown[]) => void;
}

interface MockPeerInstance {
  id: string;
  options?: unknown;
  open: boolean;
  destroyed: boolean;
  connect: ReturnType<typeof vi.fn>;
  destroy: ReturnType<typeof vi.fn>;
  reconnect: ReturnType<typeof vi.fn>;
  on: ReturnType<typeof vi.fn>;
  off: ReturnType<typeof vi.fn>;
  emit: (event: string, ...args: unknown[]) => void;
  _triggerOpen: (openId: string) => void;
  _triggerError: (error: Error) => void;
  _triggerDisconnected: () => void;
  _triggerConnection: (conn: MockConnection) => void;
}

const createMockConnection = (peerId: string, isOpen = false): MockConnection => {
  const listeners: Record<string, Array<(...args: unknown[]) => void>> = {};
  let isOpenState = isOpen;
  const conn: MockConnection = {
    peer: peerId,
    get open() { return isOpenState; },
    set open(value: boolean) { isOpenState = value; },
    send: vi.fn(),
    close: vi.fn().mockImplementation(() => { isOpenState = false; }),
    on: vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      if (!listeners[event]) listeners[event] = [];
      listeners[event].push(cb);
    }) as ReturnType<typeof vi.fn>,
    off: vi.fn((event: string, cb: (...args: unknown[]) => void) => {
      if (listeners[event]) {
        listeners[event] = listeners[event].filter((fn) => fn !== cb);
      }
    }) as ReturnType<typeof vi.fn>,
    emit: (event: string, ...args: unknown[]) => {
      if (event === "open") {
        isOpenState = true;
      }
      listeners[event]?.forEach((cb) => cb(...args));
    },
  };
  return conn;
};

// Store the last created mock connection for each peer instance
const lastMockConnectionMap = new Map<MockPeerInstance, MockConnection>();

vi.mock("peerjs", () => {
  const mockPeer = vi.fn().mockImplementation((id: string, options?: unknown) => {
    const listeners: Record<string, Array<(...args: unknown[]) => void>> = {};
    const peerInstance: MockPeerInstance = {
      id,
      options,
      open: false,
      destroyed: false,
      connect: vi.fn((peerId: string) => {
        const conn = createMockConnection(peerId);
        lastMockConnectionMap.set(peerInstance, conn);
        return conn;
      }),
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
      _triggerConnection: (conn: MockConnection) => {
        listeners["connection"]?.forEach((cb) => cb(conn));
      },
    };
    return peerInstance;
  });
  return {
    default: mockPeer,
    Peer: mockPeer,
  };
});

// Helper to get the MockPeer - uses dynamic import to get the mocked module
const getMockPeer = async () => {
  const peerjs = await import("peerjs");
  return peerjs.default as unknown as {
    (...args: unknown[]): MockPeerInstance;
    mock: { results: Array<{ value: MockPeerInstance }>; calls: Array<[string, unknown?]> };
  };
};

// Helper to get the last mock connection for a peer instance
const getLastMockConnection = (peerInstance: MockPeerInstance): MockConnection | undefined => {
  return lastMockConnectionMap.get(peerInstance);
};

// Mock cleanup utility - timers are stored but not auto-executed
// Tests manually trigger events instead of relying on timers
vi.mock("../../utils/cleanup", () => ({
  createCleanup: () => {
    const cleanupFns: Array<() => void> = [];
    let aborted = false;
    return {
      add: vi.fn((fn: () => void) => cleanupFns.push(fn)),
      setTimeout: vi.fn((_callback: () => void, _ms: number) => {
        // Don't auto-execute - tests trigger events manually
        return 0;
      }),
      setInterval: vi.fn((_callback: () => void, _ms: number) => {
        // Don't auto-execute - tests trigger events manually
        return 0;
      }),
      abort: vi.fn(() => {
        aborted = true;
        for (const fn of cleanupFns) fn();
      }),
      get aborted() { return aborted; },
    };
  },
}));

describe("P2PService integration tests", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    lastMockConnectionMap.clear();
  });

  describe("State machine transitions", () => {
    it("transitions from disconnected -> connecting -> connected", async () => {
      const service = new P2PService();
      expect(service.getState()).toBe("disconnected");

      const initPromise = service.initialize();
      const MockPeer = await getMockPeer();
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
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      await service.disconnect();
      expect(service.getState()).toBe("disconnected");
    });

    it("handles destroyed state correctly", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      await service.disconnect();

      // After disconnect, connect should fail immediately because peer is null
      await expect(service.connect("some-peer")).rejects.toThrow("Peer not initialized");
    });
  });

  describe("Connection lifecycle", () => {
    it("successfully connects to a remote peer", async () => {
      const service = new P2PService();
      const connectedHandler = vi.fn();
      service.on("connected", connectedHandler);

      const initPromise = service.initialize();
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = getLastMockConnection(peerInstance);
      expect(mockConn).toBeDefined();

      mockConn!.emit("open");
      await connectPromise;

      expect(service.isConnected()).toBe(true);
      expect(connectedHandler).toHaveBeenCalled();
    });

    it("handles incoming connections", async () => {
      const service = new P2PService();
      const peerConnectedHandler = vi.fn();
      service.on("peer_connected", peerConnectedHandler);

      const initPromise = service.initialize();
      const MockPeer = await getMockPeer();
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
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = getLastMockConnection(peerInstance);
      expect(mockConn).toBeDefined();
      mockConn!.emit("open");
      await connectPromise;

      service.sendSync({
        action: "play",
        position: 10.5,
        server_ts: Date.now(),
      });

      expect(mockConn!.send).toHaveBeenCalledWith({
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
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = getLastMockConnection(peerInstance);
      expect(mockConn).toBeDefined();
      mockConn!.emit("open");
      await connectPromise;

      const syncMessage = {
        type: "sync",
        message: {
          action: "pause",
          position: 20.0,
          server_ts: Date.now(),
        },
      };
      mockConn!.emit("data", syncMessage);

      expect(syncHandler).toHaveBeenCalledWith(syncMessage.message);
    });

    it("sends chat messages", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = getLastMockConnection(peerInstance);
      expect(mockConn).toBeDefined();
      mockConn!.emit("open");
      await connectPromise;

      service.sendChat("Hello, world!");

      expect(mockConn!.send).toHaveBeenCalledWith({
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
      const MockPeer = await getMockPeer();
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
      const MockPeer = await getMockPeer();

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
      const MockPeer = await getMockPeer();
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
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = getLastMockConnection(peerInstance);
      expect(mockConn).toBeDefined();
      mockConn!.emit("open");
      await connectPromise;

      mockConn!.emit("data", "invalid string");
      mockConn!.emit("data", { type: "unknown" });
      mockConn!.emit("data", { type: "sync" });
      mockConn!.emit("data", 123);
      mockConn!.emit("data", null);

      expect(syncHandler).not.toHaveBeenCalled();
    });

    it("cleans up all resources on disconnect", async () => {
      const service = new P2PService();

      const initPromise = service.initialize();
      const MockPeer = await getMockPeer();
      const peerInstance = MockPeer.mock.results[0]?.value;
      peerInstance._triggerOpen("test-peer-id");
      await initPromise;

      const connectPromise = service.connect("remote-peer");
      const mockConn = getLastMockConnection(peerInstance);
      expect(mockConn).toBeDefined();
      mockConn!.emit("open");
      await connectPromise;

      expect(service.isConnected()).toBe(true);

      await service.disconnect();

      expect(service.isConnected()).toBe(false);
      expect(service.isInRoom()).toBe(false);
      expect(peerInstance.destroy).toHaveBeenCalled();
    });
  });
});
