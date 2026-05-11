import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { peerInstances } = vi.hoisted(() => ({
  peerInstances: [] as unknown[],
}));

class MockRTCPeerConnection {
  createDataChannel = vi.fn(() => ({ close: vi.fn() }));
  close = vi.fn();
}

vi.mock("peerjs", () => ({
  Peer: class MockPeer {
    readonly connections: Array<{
      emitOpen: () => void;
      open: boolean;
      peer: string;
      sent: unknown[];
      close: () => void;
      on: (event: string, callback: (...args: unknown[]) => void) => void;
    }> = [];
    readonly listeners = new Map<string, Array<(...args: unknown[]) => void>>();
    open = false;
    peerId: string | null = null;

    constructor(idOrOptions?: string | Record<string, unknown>) {
      void idOrOptions;
      peerInstances.push(this as unknown);
    }

    on(event: string, callback: (...args: unknown[]) => void): void {
      const callbacks = this.listeners.get(event) ?? [];
      callbacks.push(callback);
      this.listeners.set(event, callbacks);
    }

    connect(remotePeerId: string, options: { reliable: boolean; serialization: string }) {
      void options;
      const listeners = new Map<string, Array<(...args: unknown[]) => void>>();
      const connection = {
        open: false,
        peer: remotePeerId,
        sent: [] as unknown[],
        on(event: string, callback: (...args: unknown[]) => void) {
          const callbacks = listeners.get(event) ?? [];
          callbacks.push(callback);
          listeners.set(event, callbacks);
        },
        close() {
          connection.open = false;
          for (const callback of listeners.get("close") ?? []) {
            callback();
          }
        },
        emitOpen() {
          connection.open = true;
          for (const callback of listeners.get("open") ?? []) {
            callback();
          }
        },
        send(payload: unknown) {
          connection.sent.push(payload);
        },
      };
      this.connections.push(connection);
      return connection;
    }

    destroy(): void {
      this.open = false;
      this.emit("disconnected");
    }

    emitOpen(peerId: string): void {
      this.open = true;
      this.peerId = peerId;
      this.emit("open", peerId);
    }

    private emit(event: string, ...args: unknown[]): void {
      for (const callback of this.listeners.get(event) ?? []) {
        callback(...args);
      }
    }
  },
}));

import { P2PService } from "../P2PService";

describe("P2PService connection lifecycle", () => {
  beforeEach(() => {
    peerInstances.length = 0;
    vi.stubGlobal("RTCPeerConnection", MockRTCPeerConnection);
    vi.stubGlobal("window", globalThis);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("emits connected when an outbound connection opens", async () => {
    const service = new P2PService();
    const connected = vi.fn();

    service.on("connected", connected);

    const initializePromise = service.initialize();
    const peer = peerInstances[0] as { connections: Array<{ emitOpen: () => void }>; emitOpen: (peerId: string) => void };
    peer.emitOpen("guest-peer");
    await initializePromise;

    const connectPromise = service.connect("host-peer");
    const connection = peer.connections[0];
    connection.emitOpen();
    await connectPromise;

    expect(connected).toHaveBeenCalledOnce();
    expect(service.isConnected()).toBe(true);
  });
});
