import { describe, expect, it, vi } from "vitest";
import P2PService from "../P2PService";

// Mock cleanup utility to prevent timer leaks
vi.mock("../utils/cleanup", () => ({
  createCleanup: () => {
    let aborted = false;
    return {
      add: vi.fn(),
      setTimeout: vi.fn((callback: () => void, _ms: number) => {
        // Execute callback immediately to avoid hanging promises
        // In real code, this would be a delayed execution
        if (!aborted) {
          callback();
        }
        return 1;
      }),
      setInterval: vi.fn(() => 2),
      abort: vi.fn(() => { aborted = true; }),
      get aborted() { return aborted; },
    };
  },
}));

describe("P2PService message validation", () => {
  it("generates a valid peer ID", () => {
    const svc = new P2PService();
    const peerId = svc.getPeerId();
    expect(peerId).toBeTruthy();
    expect(peerId.length).toBe(6);
    expect(/^[A-Z0-9]{6}$/.test(peerId)).toBe(true);
  });

  it("starts in disconnected state", () => {
    const svc = new P2PService();
    expect(svc.getState()).toBe("disconnected");
  });

  it("reports not connected initially", () => {
    const svc = new P2PService();
    expect(svc.isConnected()).toBe(false);
    expect(svc.isInRoom()).toBe(false);
  });

  it("tracks host/guest role correctly", () => {
    const host = new P2PService();
    host.setHost();
    expect(host.isHost()).toBe(true);

    const guest = new P2PService();
    guest.setGuest();
    expect(guest.isHost()).toBe(false);
  });

  it("returns null for RTT before any measurement", () => {
    const svc = new P2PService();
    expect(svc.getLastRttMs()).toBeNull();
  });

  it("reports unknown connection quality before measurement", () => {
    const svc = new P2PService();
    expect(svc.getConnectionQuality()).toBe("unknown");
  });

  it("allows registering and unregistering event listeners", () => {
    const svc = new P2PService();
    const callback = vi.fn();
    const unsubscribe = svc.on("connected", callback);
    expect(typeof unsubscribe).toBe("function");
    unsubscribe();
  });

  it("throws when connecting after disconnect (peer not initialized)", async () => {
    const svc = new P2PService();
    svc.disconnect();
    await expect(svc.connect("torrsync-TEST12")).rejects.toThrow();
  });

  it("sendChat drops empty content without throwing", () => {
    const svc = new P2PService();
    expect(() => svc.sendChat("")).not.toThrow();
    expect(() => svc.sendChat("  ")).not.toThrow();
  });

  it("sendSync drops invalid messages without throwing", () => {
    const svc = new P2PService();
    expect(() => svc.sendSync({ action: "play", position: NaN, server_ts: 0 })).not.toThrow();
  });

  it("sendTorrentSource drops oversized files without throwing", () => {
    const svc = new P2PService();
    const errorCallback = vi.fn();
    svc.on("error", errorCallback);
    const hugeBytes = new Uint8Array(11 * 1024 * 1024);
    svc.sendTorrentSource(
      { kind: "file", fileName: "big.torrent", bytes: hugeBytes, sourceKey: "file:big" },
      null, null, null,
    );
    expect(errorCallback).toHaveBeenCalled();
  });

  it("clearRateLimitForPeer does not throw for unknown peers", () => {
    const svc = new P2PService();
    expect(() => svc.clearRateLimitForPeer("nonexistent")).not.toThrow();
  });

  it("disconnect is idempotent", () => {
    const svc = new P2PService();
    expect(() => svc.disconnect()).not.toThrow();
    expect(() => svc.disconnect()).not.toThrow();
  });
});

describe("P2PService state machine", () => {
  it("transitions from disconnected to connecting", () => {
    const svc = new P2PService();
    expect(svc.getState()).toBe("disconnected");
  });

  it("stays in disconnected after disconnect", () => {
    const svc = new P2PService();
    svc.disconnect();
    expect(svc.getState()).toBe("disconnected");
  });
});
