import { afterEach, describe, expect, it, vi } from "vitest";
import { buildPeerServerOptions, getPeerServerOptions, getWebRTCSupportIssue } from "../P2PService";

class MockRTCDataChannel {
  close = vi.fn();
}

class MockRTCPeerConnection {
  createDataChannel = vi.fn(() => new MockRTCDataChannel());
  close = vi.fn();
}

describe("P2PService WebRTC diagnostics", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reports when RTCPeerConnection is not exposed", () => {
    vi.stubGlobal("RTCPeerConnection", undefined);

    expect(getWebRTCSupportIssue()).toBe("RTCPeerConnection is not exposed by the current WebView");
  });

  it("accepts WebViews that can create data channels", () => {
    vi.stubGlobal("RTCPeerConnection", MockRTCPeerConnection);

    expect(getWebRTCSupportIssue()).toBeNull();
  });

  it("reports constructor failures from the current WebView", () => {
    class FailingRTCPeerConnection {
      constructor() {
        throw new Error("disabled by runtime");
      }
    }

    vi.stubGlobal("RTCPeerConnection", FailingRTCPeerConnection);

    expect(getWebRTCSupportIssue()).toBe(
      "RTCPeerConnection failed to initialize: disabled by runtime",
    );
  });

  it("uses the PeerJS cloud server by default", () => {
    vi.unstubAllEnvs();

    expect(getPeerServerOptions()).toMatchObject({
      host: "0.peerjs.com",
      port: 443,
      path: "/",
      secure: true,
    });
  });

  it("allows configuring a custom PeerJS server through Vite env", () => {
    expect(
      buildPeerServerOptions({
        VITE_PEERJS_HOST: "signal.example.test",
        VITE_PEERJS_PORT: "9000",
        VITE_PEERJS_PATH: "peer",
        VITE_PEERJS_SECURE: "false",
      }),
    ).toMatchObject({
      host: "signal.example.test",
      port: 9000,
      path: "/peer/",
      secure: false,
    });
  });
});
