import type { AudioTrackInfo, SubtitleTrackInfo } from "../services/types";

export interface ElectronTorrentFile {
  index?: number;
  name: string;
  length?: number;
  progress?: number;
  streamUrl?: string;
  kind?: string;
  extension?: string;
  streamTo?: (mediaElement: HTMLMediaElement) => Promise<void>;
  blob?: () => Promise<Blob>;
}

export interface ElectronTorrentInstance {
  files: ElectronTorrentFile[];
  progress: number;
  downloadSpeed: number;
  numPeers: number;
  discoveredPeerCount?: number;
  on?: (event: string, callback: (...args: unknown[]) => void) => void;
  destroy?: (callback?: (error?: Error) => void) => void;
  select?: (start: number, end: number, priority: number) => void;
  deselect?: (start: number, end: number, priority: number) => void;
  downloaded?: number;
  pause?: () => void;
  resume?: () => void;
  paused?: boolean;
}

export interface ElectronTorrentBackend {
  addMagnet: (magnetLink: string) => Promise<ElectronTorrentInstance>;
  addTorrentFile: (torrentFile: Uint8Array) => Promise<ElectronTorrentInstance>;
  getStats: () => Promise<ElectronTorrentInstance | null>;
  clear: () => Promise<void>;
  setMaxBufferMB?: (mb: number) => Promise<void>;
  probeAudioTracks?: (streamUrl: string) => Promise<AudioTrackInfo[]>;
  probeSubtitles?: (streamUrl: string) => Promise<SubtitleTrackInfo[]>;
  createAudioTrackStreamUrl?: (params: {
    streamUrl: string;
    trackIndex: number;
    startSeconds: number;
  }) => Promise<string>;
  createSubtitleStreamUrl?: (params: {
    streamUrl: string;
    trackIndex: number;
    startSeconds: number;
  }) => Promise<string>;
  createMultiplexedStreamUrl?: (params: {
    streamUrl: string;
    audioTrackIndex: number;
    startSeconds: number;
  }) => Promise<string>;
  isFfmpegAvailable?: () => Promise<boolean>;
}

export interface ElectronWindow {
  torrsyncElectronTorrent?: ElectronTorrentBackend;
  torrsyncElectronWindow?: {
    onCloseRequest: (callback: () => void) => void;
    closeConfirmed: () => void;
    closeCancelled: () => void;
  };
}

declare global {
  interface Window {
    torrsyncElectronTorrent?: ElectronTorrentBackend;
    torrsyncElectronWindow?: ElectronWindow["torrsyncElectronWindow"];
  }
}
