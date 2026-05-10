import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PeerService } from "../PeerService";

class MockRTCSessionDescription {
  readonly type?: RTCSdpType;
  readonly sdp?: string;

  constructor(init: RTCSessionDescriptionInit) {
    this.type = init.type;
    this.sdp = init.sdp;
  }
}

class MockRTCIceCandidate {
  readonly candidate?: string;

  constructor(init: RTCIceCandidateInit) {
    this.candidate = init.candidate;
  }
}

class MockDataChannel {
  readyState: RTCDataChannelState = "open";
  sent: string[] = [];
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.readyState = "closed";
    this.onclose?.();
  }

  receive(data: string): void {
    this.onmessage?.({ data });
  }
}

class MockRTCPeerConnection {
  static instances: MockRTCPeerConnection[] = [];

  connectionState: RTCPeerConnectionState = "new";
  localDescription: RTCSessionDescriptionInit | null = null;
  remoteDescription: RTCSessionDescriptionInit | null = null;
  createdChannel: MockDataChannel | null = null;
  onicecandidate: ((event: { candidate: { toJSON: () => RTCIceCandidateInit } | null }) => void) | null =
    null;
  ondatachannel: ((event: { channel: MockDataChannel }) => void) | null = null;
  onconnectionstatechange: (() => void) | null = null;
  createDataChannel = vi.fn(() => {
    this.createdChannel = new MockDataChannel();
    return this.createdChannel;
  });
  createOffer = vi.fn(async () => ({ type: "offer" as const, sdp: "offer-sdp" }));
  createAnswer = vi.fn(async () => ({ type: "answer" as const, sdp: "answer-sdp" }));
  setLocalDescription = vi.fn(async (description: RTCSessionDescriptionInit) => {
    this.localDescription = description;
  });
  setRemoteDescription = vi.fn(async (description: RTCSessionDescriptionInit) => {
    this.remoteDescription = description;
  });
  addIceCandidate = vi.fn(async (_candidate: MockRTCIceCandidate) => undefined);
  close = vi.fn(() => {
    this.connectionState = "closed";
    this.onconnectionstatechange?.();
  });

  constructor() {
    MockRTCPeerConnection.instances.push(this);
  }

  emitIce(candidate: RTCIceCandidateInit): void {
    this.onicecandidate?.({ candidate: { toJSON: () => candidate } });
  }

  emitDataChannel(channel = new MockDataChannel()): MockDataChannel {
    this.ondatachannel?.({ channel });
    return channel;
  }
}

describe("PeerService", () => {
  beforeEach(() => {
    MockRTCPeerConnection.instances = [];
    vi.stubGlobal("RTCPeerConnection", MockRTCPeerConnection);
    vi.stubGlobal("RTCSessionDescription", MockRTCSessionDescription);
    vi.stubGlobal("RTCIceCandidate", MockRTCIceCandidate);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("creates offers, stores data channels, and emits channel events", async () => {
    const service = new PeerService();
    const open = vi.fn();
    const data = vi.fn();
    const ice = vi.fn();
    service.on("open", open);
    service.on("data", data);
    service.on("ice", ice);

    const offer = await service.createConnection("peer-a");
    const connection = MockRTCPeerConnection.instances[0];
    const channel = connection.createdChannel;

    expect(offer).toEqual({ type: "offer", sdp: "offer-sdp" });
    expect(connection.setLocalDescription).toHaveBeenCalledWith(offer);
    expect(channel).not.toBeNull();

    channel?.onopen?.();
    channel?.receive(JSON.stringify({ action: "play" }));
    channel?.receive("plain-text");
    connection.emitIce({ candidate: "ice-candidate" });
    service.sendData("peer-a", { action: "pause" });

    expect(open).toHaveBeenCalledWith("peer-a");
    expect(data).toHaveBeenNthCalledWith(1, { peerId: "peer-a", data: { action: "play" } });
    expect(data).toHaveBeenNthCalledWith(2, { peerId: "peer-a", data: "plain-text" });
    expect(ice).toHaveBeenCalledWith({ peerId: "peer-a", candidate: { candidate: "ice-candidate" } });
    expect(channel?.sent).toEqual([JSON.stringify({ action: "pause" })]);
  });

  it("handles offers, answers, and ICE candidates on existing connections", async () => {
    const service = new PeerService();

    const answer = await service.handleOffer("peer-a", { type: "offer", sdp: "remote-offer" });
    await service.handleAnswer("peer-a", { type: "answer", sdp: "remote-answer" });
    await service.handleIce("peer-a", { candidate: "remote-ice" });

    const connection = MockRTCPeerConnection.instances[0];
    expect(answer).toEqual({ type: "answer", sdp: "answer-sdp" });
    expect(connection.setRemoteDescription).toHaveBeenNthCalledWith(1, {
      type: "offer",
      sdp: "remote-offer",
    });
    expect(connection.setRemoteDescription).toHaveBeenNthCalledWith(2, {
      type: "answer",
      sdp: "remote-answer",
    });
    expect(connection.addIceCandidate).toHaveBeenCalledWith(expect.objectContaining({ candidate: "remote-ice" }));
  });

  it("closes peer resources and emits close events", async () => {
    const service = new PeerService();
    const close = vi.fn();
    service.on("close", close);

    await service.createConnection("peer-a");
    service.close("peer-a");

    expect(close).toHaveBeenCalledWith("peer-a");
    expect(MockRTCPeerConnection.instances[0].close).toHaveBeenCalledOnce();
  });
});
