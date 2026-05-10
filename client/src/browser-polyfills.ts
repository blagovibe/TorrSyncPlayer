import { Buffer } from "buffer";

type BrowserGlobal = typeof globalThis & {
  Buffer?: typeof Buffer;
  global?: typeof globalThis;
  process?: {
    browser?: boolean;
    env: Record<string, string | undefined>;
    nextTick?: (callback: (...args: unknown[]) => void, ...args: unknown[]) => void;
    version?: string;
  };
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
