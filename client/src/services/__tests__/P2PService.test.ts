import { afterEach, describe, expect, it, vi } from "vitest";
import { getWebRTCSupportIssue } from "../P2PService";

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
});
