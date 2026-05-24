/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_PEERJS_HOST: string;
  readonly VITE_PEERJS_PORT: string;
  readonly VITE_PEERJS_PATH: string;
  readonly VITE_PEERJS_SECURE: string;
  readonly VITE_WEBTORRENT_TRACKERS: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare module "webtorrent" {
  interface TorrentFile {
    name: string;
    length: number;
    progress: number;
    streamUrl?: string;
    streamTo: (mediaElement: HTMLMediaElement) => Promise<void>;
    blob?: () => Promise<Blob>;
    index?: number;
  }

  interface Torrent {
    files: TorrentFile[];
    progress: number;
    downloadSpeed: number;
    numPeers: number;
    discoveredPeerCount?: number;
    on: (event: string, callback: (...args: unknown[]) => void) => void;
    destroy: (callback?: (error?: Error) => void) => void;
    select: (start: number, end: number, priority: number) => void;
    deselect: (start: number, end: number, priority: number) => void;
    downloaded?: number;
    paused?: boolean;
    pause?: () => void;
    resume?: () => void;
  }

  interface WebTorrentOptions {
    maxConns?: number;
    tracker?: {
      announce?: string[];
    };
    sequential?: boolean;
  }

  interface StreamServer {
    listen: (port?: number, host?: string, callback?: () => void) => void;
    address: () => { port: number } | string | null;
    close: () => void;
    server?: { close: () => void };
  }

  class WebTorrent {
    constructor(options?: WebTorrentOptions);
    add: (source: string | Uint8Array, opts?: Record<string, unknown>, callback?: (torrent: Torrent) => void) => Torrent;
    createServer: (options?: { controller?: ServiceWorkerRegistration; origin?: string }) => StreamServer;
    destroy: (callback?: (error?: Error) => void) => void;
    _server?: { close: () => void };
  }

  export default WebTorrent;
}
