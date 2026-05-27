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
        emitData(data: unknown) {
          for (const callback of listeners.get("data") ?? []) {
            callback(data);
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

  it("emits disconnected when connection closes and can disconnect cleanly", async () => {
    const service = new P2PService();
    const connected = vi.fn();
    const disconnected = vi.fn();
    const peerConnected = vi.fn();
    const peerDisconnected = vi.fn();

    service.on("connected", connected);
    service.on("disconnected", disconnected);
    service.on("peer_connected", peerConnected);
    service.on("peer_disconnected", peerDisconnected);

    const initializePromise = service.initialize();
    const peer = peerInstances[0] as { connections: Array<{ emitOpen: () => void; close: () => void; emitData: (d: unknown) => void }>; emitOpen: (peerId: string) => void };
    peer.emitOpen("guest-peer");
    await initializePromise;

    const connectPromise = service.connect("host-peer");
    const connection = peer.connections[0];
    connection.emitOpen();
    await connectPromise;

    expect(connected).toHaveBeenCalledOnce();
    expect(peerConnected).toHaveBeenCalledWith("host-peer");
    expect(service.isConnected()).toBe(true);

    connection.close();
    expect(disconnected).toHaveBeenCalled();
    expect(peerDisconnected).toHaveBeenCalledWith("host-peer");
    expect(service.isConnected()).toBe(false);
  });

  it("disconnect is idempotent and resets all state", async () => {
    const service = new P2PService();

    const initializePromise = service.initialize();
    const peer = peerInstances[0] as { connections: Array<{ emitOpen: () => void; close: () => void }>; emitOpen: (peerId: string) => void };
    peer.emitOpen("guest-peer");
    await initializePromise;

    const connectPromise = service.connect("host-peer");
    const connection = peer.connections[0];
    connection.emitOpen();
    await connectPromise;

    expect(service.isInRoom()).toBe(true);

    await service.disconnect();
    expect(service.isConnected()).toBe(false);
    expect(service.isInRoom()).toBe(false);

    await service.disconnect();
    expect(service.isConnected()).toBe(false);
  });

  it("can reconnect after disconnect by re-initializing", async () => {
    const service = new P2PService();
    const connected1 = vi.fn();
    const disconnected1 = vi.fn();
    const connected2 = vi.fn();

    service.on("connected", connected1);
    service.on("disconnected", disconnected1);

    const init1 = service.initialize();
    const peer1 = peerInstances[peerInstances.length - 1] as { emitOpen: (id: string) => void; connections: Array<{ emitOpen: () => void }> };
    peer1.emitOpen("peer-1");
    await init1;

    const conn1 = service.connect("remote-1");
    peer1.connections[0].emitOpen();
    await conn1;
    expect(connected1).toHaveBeenCalledTimes(1);

    await service.disconnect();
    expect(disconnected1).toHaveBeenCalled();

    service.on("connected", connected2);
    const init2 = service.initialize();
    const peer2 = peerInstances[peerInstances.length - 1] as { emitOpen: (id: string) => void; connections: Array<{ emitOpen: () => void }> };
    peer2.emitOpen("peer-2");
    await init2;

    const conn2 = service.connect("remote-2");
    peer2.connections[0].emitOpen();
    await conn2;
    expect(connected2).toHaveBeenCalledTimes(1);
  });

  it("keeps selectedAudioTrackIndex in torrent source messages", async () => {
    const service = new P2PService();
    const received = vi.fn();

    service.on("torrent_source", received);

    const initializePromise = service.initialize();
    const peer = peerInstances[0] as {
      connections: Array<{
        emitOpen: () => void;
        emitData: (data: unknown) => void;
        sent: unknown[];
      }>;
      emitOpen: (peerId: string) => void;
    };
    peer.emitOpen("host-peer");
    await initializePromise;

    const connectPromise = service.connect("guest-peer");
    const connection = peer.connections[0];
    connection.emitOpen();
    await connectPromise;

    const source = {
      kind: "magnet",
      magnetLink: "magnet:?xt=urn:btih:test",
      sourceKey: "magnet:magnet:?xt=urn:btih:test",
    } as const;

     service.sendTorrentSource(
       source,
       4,
       2,
       null,
     );

    expect(connection.sent).toEqual([
      {
        type: "torrent_source",
        source,
        selectedMediaIndex: 4,
        selectedAudioTrackIndex: 2,
        selectedSubtitleIndex: null,
      },
    ]);

    connection.emitData(
      JSON.stringify({
        type: "torrent_source",
        source,
        selectedMediaIndex: 1,
        selectedAudioTrackIndex: 3,
        selectedSubtitleIndex: null,
      }),
    );

    expect(received).toHaveBeenCalledWith({
      source,
      selectedMediaIndex: 1,
      selectedAudioTrackIndex: 3,
      selectedSubtitleIndex: null,
    });
  });
});
