declare module "webtorrent" {
  interface TorrentFile {
    name: string;
    length: number;
    progress: number;
    streamUrl?: string;
    streamTo: (mediaElement: HTMLMediaElement) => Promise<void>;
    blob?: () => Promise<Blob>;
  }

  interface Torrent {
    files: TorrentFile[];
    progress: number;
    downloadSpeed: number;
    numPeers: number;
    on: (event: string, callback: (...args: unknown[]) => void) => void;
    destroy: (callback?: (error?: Error) => void) => void;
    select: (start: number, end: number, priority: number) => void;
    deselect: (start: number, end: number, priority: number) => void;
  }

  interface WebTorrentOptions {
    maxConns?: number;
    tracker?: {
      announce?: string[];
    };
  }

  class WebTorrent {
    constructor(options?: WebTorrentOptions);
    add: (source: string | Uint8Array, callback?: (torrent: Torrent) => void) => Torrent;
    createServer: (options?: { controller?: ServiceWorkerRegistration; origin?: string }) => { listen: (port: number, host: string, callback?: () => void) => void; address: () => { port: number } | string | null; close: () => void; server?: { close: () => void } };
    destroy: (callback?: (error?: Error) => void) => void;
    _server?: { close: () => void };
  }

  export default WebTorrent;
}
