/**
 * Unified cleanup pattern using AbortController.
 *
 * All services use this pattern for consistent resource management:
 * - Timers (setTimeout, setInterval)
 * - Event listeners (addEventListener, .on())
 * - Subscriptions
 *
 * Usage:
 * ```ts
 * class MyService {
 *   private cleanup = createCleanup();
 *
 *   start() {
 *     this.cleanup.setTimeout(() => {}, 1000);
 *     this.cleanup.addEventListener(window, 'resize', handler);
 *     this.cleanup.on(emitter, 'event', callback);
 *   }
 *
 *   destroy() {
 *     this.cleanup.abort(); // Cancels everything
 *   }
 * }
 * ```
 */

export interface CleanupHandle {
  /** Cancel a specific timer by its ID */
  clearTimer(id: ReturnType<typeof setTimeout>): void;
  /** Add a managed setTimeout */
  setTimeout(callback: () => void, ms: number): ReturnType<typeof setTimeout>;
  /** Add a managed setInterval */
  setInterval(callback: () => void, ms: number): ReturnType<typeof setInterval>;
  /** Add a managed event listener */
  addEventListener<K extends keyof WindowEventMap>(
    target: Window,
    type: K,
    listener: (this: Window, ev: WindowEventMap[K]) => void,
    options?: boolean | AddEventListenerOptions,
  ): void;
  addEventListener(
    target: EventTarget,
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | AddEventListenerOptions,
  ): void;
  /** Remove a specific event listener */
  removeEventListener(
    target: EventTarget,
    type: string,
    listener: EventListenerOrEventListenerObject,
  ): void;
  /** Add a managed .on() subscription (for EventEmitter-like objects) */
  on(
    emitter: {
      on: (event: string, cb: (...args: unknown[]) => void) => void;
      off?: (event: string, cb: (...args: unknown[]) => void) => void;
      removeListener?: (event: string, cb: (...args: unknown[]) => void) => void;
    },
    event: string,
    callback: (...args: unknown[]) => void,
  ): void;
  /** Register an arbitrary cleanup function */
  add(fn: () => void): void;
  /** Check if already aborted */
  readonly aborted: boolean;
  /** The underlying AbortSignal */
  readonly signal: AbortSignal;
  /** Manually trigger cleanup */
  abort(): void;
}

export function createCleanup(): CleanupHandle {
  const abortController = new AbortController();
  const signal = abortController.signal;
  const timers: ReturnType<typeof setTimeout>[] = [];
  const listeners: Array<{
    target: EventTarget;
    type: string;
    listener: EventListenerOrEventListenerObject;
  }> = [];
  const subscriptions: Array<{
    emitter: {
      off?: (event: string, cb: (...args: unknown[]) => void) => void;
      removeListener?: (event: string, cb: (...args: unknown[]) => void) => void;
    };
    event: string;
    callback: (...args: unknown[]) => void;
  }> = [];
  const cleanupFns: Array<() => void> = [];

  function clearTimer(id: ReturnType<typeof setTimeout>) {
    clearTimeout(id);
    const idx = timers.indexOf(id);
    if (idx !== -1) timers.splice(idx, 1);
  }

  function removeAllTimers() {
    for (let i = 0; i < timers.length; i++) {
      clearTimeout(timers[i]);
    }
    timers.length = 0;
  }

  function removeAllListeners() {
    for (let i = 0; i < listeners.length; i++) {
      const l = listeners[i];
      l.target.removeEventListener(l.type, l.listener);
    }
    listeners.length = 0;
  }

  function removeAllSubscriptions() {
    for (let i = 0; i < subscriptions.length; i++) {
      const s = subscriptions[i];
      if (s.emitter.off) {
        s.emitter.off(s.event, s.callback);
      } else if (s.emitter.removeListener) {
        s.emitter.removeListener(s.event, s.callback);
      }
    }
    subscriptions.length = 0;
  }

  function runCleanupFns() {
    for (let i = 0; i < cleanupFns.length; i++) {
      try {
        cleanupFns[i]();
      } catch {
        // Ignore cleanup errors
      }
    }
    cleanupFns.length = 0;
  }

  function cleanup() {
    removeAllTimers();
    removeAllListeners();
    removeAllSubscriptions();
    runCleanupFns();
  }

  const handle: CleanupHandle = {
    get aborted() {
      return signal.aborted;
    },
    signal,

    abort() {
      if (signal.aborted) return;
      abortController.abort();
    },

    clearTimer(id: ReturnType<typeof setTimeout>) {
      clearTimer(id);
    },

    setTimeout(callback: () => void, ms: number) {
      const id = setTimeout(() => {
        clearTimer(id);
        if (!signal.aborted) callback();
      }, ms);
      timers.push(id);
      return id;
    },

    setInterval(callback: () => void, ms: number) {
      const id = setInterval(() => {
        if (!signal.aborted) callback();
      }, ms);
      timers.push(id);
      return id;
    },

    addEventListener(
      target: EventTarget,
      type: string,
      listener: EventListenerOrEventListenerObject,
      options?: boolean | AddEventListenerOptions,
    ) {
      target.addEventListener(type, listener, options);
      listeners.push({ target, type, listener });
    },

    removeEventListener(
      target: EventTarget,
      type: string,
      listener: EventListenerOrEventListenerObject,
    ) {
      target.removeEventListener(type, listener);
      for (let i = 0; i < listeners.length; i++) {
        if (
          listeners[i].target === target &&
          listeners[i].type === type &&
          listeners[i].listener === listener
        ) {
          listeners.splice(i, 1);
          break;
        }
      }
    },

    on(
      emitter: {
        on: (event: string, cb: (...args: unknown[]) => void) => void;
        off?: (event: string, cb: (...args: unknown[]) => void) => void;
        removeListener?: (event: string, cb: (...args: unknown[]) => void) => void;
      },
      event: string,
      callback: (...args: unknown[]) => void,
    ) {
      emitter.on(event, callback);
      subscriptions.push({ emitter, event, callback });
    },

    add(fn: () => void) {
      cleanupFns.push(fn);
    },
  };

  // Listen for abort to clean everything up
  signal.addEventListener('abort', cleanup, { once: true });

  return handle;
}
