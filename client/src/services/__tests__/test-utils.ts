import { vi } from "vitest";

type TorrentEvent = "download" | "metadata" | "ready" | "error" | "wire" | "noPeers" | "peer";
type TorrentCallback = (...args: unknown[]) => void | Promise<void>;

export function createTorrent(
  files: Array<{ name: string; length?: number; streamTo?: (video: HTMLMediaElement) => Promise<void> }>,
) {
  const listeners = new Map<TorrentEvent, TorrentCallback>();
  const torrent = {
    files: files.map((file) => ({
      streamTo: vi.fn().mockResolvedValue(undefined),
      length: 1024,
      ...file,
    })),
    progress: 0.35,
    downloadSpeed: 2048,
    numPeers: 0,
    on: vi.fn((event: string, callback: TorrentCallback) => {
      listeners.set(event as TorrentEvent, callback);
    }),
    emit: async (event: string, ...args: unknown[]) => {
      if (event === "wire") {
        torrent.numPeers += 1;
      }
      await listeners.get(event as TorrentEvent)?.(...args);
    },
  };

  return torrent;
}

export function setupElectronBackendCleanup() {
  if (typeof window !== "undefined") {
    delete (window as Window & { torrsyncElectronTorrent?: unknown }).torrsyncElectronTorrent;
  }
}
