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
  process?: BrowserProcess;
  webkitRTCIceCandidate?: typeof RTCIceCandidate;
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

if (!browserGlobal.RTCSessionDescription && browserGlobal.webkitRTCSessionDescription) {
  browserGlobal.RTCSessionDescription = browserGlobal.webkitRTCSessionDescription;
}

if (!browserGlobal.RTCIceCandidate && browserGlobal.webkitRTCIceCandidate) {
  browserGlobal.RTCIceCandidate = browserGlobal.webkitRTCIceCandidate;
}
