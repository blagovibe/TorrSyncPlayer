/**
 * @fileoverview Lazy loader for WebTorrent client module.
 *
 * Implements dynamic import with caching, preloading, and error handling
 * to reduce initial bundle size and improve startup performance.
 *
 * @example
 * ```typescript
 * // Lazy load WebTorrent only when needed
 * const client = await WebTorrentLoader.getClient();
 *
 * // Preload in background for faster subsequent use
 * WebTorrentLoader.preload();
 * ```
 */

import { torrentLogger } from "../utils/logger";
import type { TorrentInstance } from "./torrent-backend";

/** Torrent source type - magnet link or raw bytes */
type TorrentSource = string | Uint8Array;

/** Torrent client interface - subset of WebTorrent API we use */
export type TorrentClient = {
  add: (torrentSource: TorrentSource, opts?: Record<string, unknown>, callback?: (torrent: TorrentInstance) => void) => TorrentInstance | null;
  destroy: (callback?: () => void) => void;
  createServer?: (options: { controller: ServiceWorkerRegistration }) => unknown;
  _server?: { close?: () => void };
};

/** WebTorrent constructor type */
type WebTorrentConstructor = new (options?: Record<string, unknown>) => TorrentClient;

/** Cached WebTorrent module */
let cachedModule: { default: WebTorrentConstructor } | null = null;

/** Promise for in-flight import to avoid duplicate loads */
let importPromise: Promise<{ default: WebTorrentConstructor }> | null = null;

/**
 * Lazy load WebTorrent module with caching.
 * Uses dynamic import to split WebTorrent into a separate chunk.
 * 
 * @returns Promise resolving to WebTorrent constructor
 * @throws If WebTorrent module fails to load
 */
export async function loadWebTorrent(): Promise<WebTorrentConstructor> {
  // Return cached module if available
  if (cachedModule) {
    return cachedModule.default;
  }

  // Return in-flight promise if import is already happening
  if (importPromise) {
    const mod = await importPromise;
    return mod.default;
  }

  // Start dynamic import with caching
  torrentLogger.info("Lazy loading WebTorrent module...");
  const startTime = performance.now();

  importPromise = import("webtorrent") as Promise<{ default: WebTorrentConstructor }>;

  try {
    cachedModule = await importPromise;
    const loadTime = Math.round(performance.now() - startTime);
    torrentLogger.info(`WebTorrent module loaded in ${loadTime}ms`);
    return cachedModule.default;
  } catch (error) {
    torrentLogger.error("Failed to load WebTorrent module:", error);
    throw new Error(
      `Failed to load WebTorrent: ${error instanceof Error ? error.message : String(error)}`
    );
  } finally {
    importPromise = null;
  }
}

/**
 * Preload WebTorrent module in background.
 * Call this early to have WebTorrent ready when needed.
 * Safe to call multiple times - only triggers one load.
 * 
 * @returns Promise that resolves when preload completes (or rejects silently)
 */
export function preloadWebTorrent(): void {
  if (cachedModule || importPromise) {
    return; // Already loaded or loading
  }

  // Start loading in background, don't await
  loadWebTorrent().catch((error) => {
    // Silently log preload failures - will retry on actual use
    torrentLogger.debug("WebTorrent preload failed, will retry on use:", error);
  });
}

/**
 * Check if WebTorrent module is already loaded and cached.
 * 
 * @returns true if module is cached and ready to use
 */
export function isWebTorrentLoaded(): boolean {
  return cachedModule !== null;
}

/**
 * Clear cached WebTorrent module.
 * Useful for testing or memory cleanup.
 */
export function clearWebTorrentCache(): void {
  cachedModule = null;
  importPromise = null;
}

/**
 * Get loading progress/status for UI indicators.
 * 
 * @returns Object with loading state information
 */
export function getWebTorrentLoadStatus(): {
  loaded: boolean;
  loading: boolean;
} {
  return {
    loaded: cachedModule !== null,
    loading: importPromise !== null,
  };
}

export default {
  load: loadWebTorrent,
  preload: preloadWebTorrent,
  isLoaded: isWebTorrentLoaded,
  clearCache: clearWebTorrentCache,
  getStatus: getWebTorrentLoadStatus,
};
