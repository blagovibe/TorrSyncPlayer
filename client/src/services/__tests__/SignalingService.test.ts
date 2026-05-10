import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SignalingService } from "../SignalingService";

class MockWebSocket {
  static readonly OPEN = 1;
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readyState = MockWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  send = vi.fn();
  close = vi.fn(() => {
    this.readyState = 3;
    this.onclose?.();
  });

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  open(): void {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  receive(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  receiveRaw(data: string): void {
    this.onmessage?.({ data });
  }
}

describe("SignalingService", () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal("WebSocket", MockWebSocket);
    vi.stubGlobal("window", globalThis);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("connects, emits lifecycle events, and sends room messages when open", () => {
    const service = new SignalingService("ws://test");
    const connected = vi.fn();
    const disconnected = vi.fn();

    service.on("connected", connected);
    service.on("disconnected", disconnected);
    service.connect();

    const socket = MockWebSocket.instances[0];
    expect(socket.url).toBe("ws://test");

    socket.open();
    service.createRoom();
    service.joinRoom("ROOM42");
    service.leaveRoom();

    expect(connected).toHaveBeenCalledOnce();
    expect(socket.send).toHaveBeenNthCalledWith(1, JSON.stringify({ type: "create_room" }));
    expect(socket.send).toHaveBeenNthCalledWith(
      2,
      JSON.stringify({ type: "join_room", roomCode: "ROOM42" }),
    );
    expect(socket.send).toHaveBeenNthCalledWith(3, JSON.stringify({ type: "leave_room" }));
    expect(disconnected).toHaveBeenCalledOnce();
  });

  it("routes inbound signaling messages and supports listener cleanup", () => {
    const service = new SignalingService("ws://test");
    const peerJoined = vi.fn();
    const peerLeft = vi.fn();
    const offer = vi.fn();
    const answer = vi.fn();
    const ice = vi.fn();
    const sync = vi.fn();
    const roomCreated = vi.fn();
    const joined = vi.fn();
    const removed = vi.fn();

    service.on("peer_joined", peerJoined);
    service.on("peer_left", peerLeft);
    service.on("offer", offer);
    service.on("answer", answer);
    service.on("ice", ice);
    service.on("sync", sync);
    service.on("room_created", roomCreated);
    service.on("joined", joined);
    const unsubscribe = service.on("peer_joined", removed);
    unsubscribe();

    service.connect();
    const socket = MockWebSocket.instances[0];
    socket.receive({ type: "peer_joined", peerId: "peer-a" });
    socket.receive({ type: "peer_left", peerId: "peer-b" });
    socket.receive({ type: "offer", from: "peer-a", sdp: { type: "offer", sdp: "sdp" } });
    socket.receive({ type: "answer", from: "peer-b", sdp: { type: "answer", sdp: "sdp" } });
    socket.receive({ type: "ice", from: "peer-c", candidate: { candidate: "ice" } });
    socket.receive({ type: "sync", from: "peer-d", action: "seek", position: 12, server_ts: 50 });
    socket.receive({ type: "room_created", code: "ABCD" });
    socket.receive({ type: "joined", code: "ABCD", peers: ["peer-a"] });
    socket.receiveRaw("not-json");

    expect(peerJoined).toHaveBeenCalledWith("peer-a");
    expect(removed).not.toHaveBeenCalled();
    expect(peerLeft).toHaveBeenCalledWith("peer-b");
    expect(offer).toHaveBeenCalledWith({ from: "peer-a", sdp: { type: "offer", sdp: "sdp" } });
    expect(answer).toHaveBeenCalledWith({ from: "peer-b", sdp: { type: "answer", sdp: "sdp" } });
    expect(ice).toHaveBeenCalledWith({ from: "peer-c", candidate: { candidate: "ice" } });
    expect(sync).toHaveBeenCalledWith({
      from: "peer-d",
      action: "seek",
      position: 12,
      server_ts: 50,
    });
    expect(roomCreated).toHaveBeenCalledWith({ code: "ABCD" });
    expect(joined).toHaveBeenCalledWith({ code: "ABCD", peers: ["peer-a"] });
  });

  it("reconnects after an unplanned close with exponential backoff", () => {
    vi.useFakeTimers();
    const service = new SignalingService("ws://test");

    service.connect();
    MockWebSocket.instances[0].close();

    expect(MockWebSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(999);
    expect(MockWebSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(MockWebSocket.instances).toHaveLength(2);
  });

  it("does not send messages before the socket is open", () => {
    const service = new SignalingService("ws://test");
    service.createRoom();

    service.connect();
    const socket = MockWebSocket.instances[0];
    socket.readyState = 0;
    service.joinRoom("ROOM42");

    expect(socket.send).not.toHaveBeenCalled();
  });
});
