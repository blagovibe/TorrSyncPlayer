import { Buffer } from "buffer";

type BrowserProcess = {
  browser?: boolean;
  env: Record<string, string | undefined>;
  nextTick?: (callback: (...args: unknown[]) => void, ...args: unknown[]) => void;
  version?: string;
};

type BrowserGlobal = Omit<typeof globalThis, "global" | "process"> & {
  Buffer?: typeof Buffer;
  global?: BrowserGlobal;
  mozRTCPeerConnection?: typeof RTCPeerConnection;
  process?: BrowserProcess;
  webkitRTCIceCandidate?: typeof RTCIceCandidate;
  webkitRTCPeerConnection?: typeof RTCPeerConnection;
  webkitRTCSessionDescription?: typeof RTCSessionDescription;
};

const browserGlobal = globalThis as BrowserGlobal;

browserGlobal.global ??= browserGlobal;
browserGlobal.Buffer ??= Buffer;
browserGlobal.process ??= {
  browser: true,
  env: {},
  nextTick: (callback, ...args) => queueMicrotask(() => callback(...args)),
  version: "",
};

if (browserGlobal.process) {
  browserGlobal.process.env ??= {};
  browserGlobal.process.browser = true;
  browserGlobal.process.nextTick ??= (callback, ...args) => queueMicrotask(() => callback(...args));
  browserGlobal.process.version ??= "";
}

const nativeRTCPeerConnection =
  browserGlobal.RTCPeerConnection ??
  browserGlobal.webkitRTCPeerConnection ??
  browserGlobal.mozRTCPeerConnection;

if (nativeRTCPeerConnection) {
  browserGlobal.RTCPeerConnection = function RTCPeerConnection(
    configuration?: RTCConfiguration,
    ...args: unknown[]
  ) {
    try {
      return Reflect.construct(nativeRTCPeerConnection, [configuration, ...args], new.target);
    } catch (error) {
      if (!configuration || !("sdpSemantics" in configuration)) {
        throw error;
      }

      const { sdpSemantics: _sdpSemantics, ...compatibleConfiguration } =
        configuration as RTCConfiguration & { sdpSemantics?: unknown };
      return Reflect.construct(nativeRTCPeerConnection, [compatibleConfiguration, ...args], new.target);
    }
  } as unknown as typeof RTCPeerConnection;

  browserGlobal.RTCPeerConnection.prototype = nativeRTCPeerConnection.prototype;
  Object.setPrototypeOf(browserGlobal.RTCPeerConnection, nativeRTCPeerConnection);
}

if (!browserGlobal.RTCSessionDescription && browserGlobal.webkitRTCSessionDescription) {
  browserGlobal.RTCSessionDescription = browserGlobal.webkitRTCSessionDescription;
}

if (!browserGlobal.RTCIceCandidate && browserGlobal.webkitRTCIceCandidate) {
  browserGlobal.RTCIceCandidate = browserGlobal.webkitRTCIceCandidate;
}
