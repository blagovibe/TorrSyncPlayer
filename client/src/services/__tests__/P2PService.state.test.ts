// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import P2PService from "../P2PService";

Object.defineProperty(globalThis, "RTCPeerConnection", {
  value: class MockRTCPeerConnection {
    createDataChannel = vi.fn().mockReturnValue({ close: vi.fn() });
    close = vi.fn();
  },
  writable: true,
  configurable: true,
});

describe("P2PService state machine", () => {
  it("starts in disconnected state", () => {
    const service = new P2PService();
    expect(service.getState()).toBe("disconnected");
  });

  it("is not connected initially", () => {
    const service = new P2PService();
    expect(service.isConnected()).toBe(false);
  });

  it("isInRoom returns false when not initialized", () => {
    const service = new P2PService();
    expect(service.isInRoom()).toBe(false);
  });

  it("returns peer id", () => {
    const service = new P2PService();
    const id = service.getPeerId();
    expect(id).toBeTruthy();
    expect(id.length).toBe(6);
  });

  it("setHost and isHost work", () => {
    const service = new P2PService();
    expect(service.isHost()).toBe(true);
    service.setGuest();
    expect(service.isHost()).toBe(false);
    service.setHost();
    expect(service.isHost()).toBe(true);
  });

  it("getLastRttMs returns null initially", () => {
    const service = new P2PService();
    expect(service.getLastRttMs()).toBeNull();
  });

  it("getConnectionQuality returns unknown initially", () => {
    const service = new P2PService();
    expect(service.getConnectionQuality()).toBe("unknown");
  });

  it("can register event listeners", () => {
    const service = new P2PService();
    const unsub = service.on("connected", () => {});
    expect(typeof unsub).toBe("function");
    unsub();
  });
});
