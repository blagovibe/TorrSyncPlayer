import type { AudioTrackInfo, SubtitleTrackInfo } from "./types";
import type { ElectronTorrentBackend, ElectronTorrentFile, ElectronTorrentInstance, ElectronWindow } from "../types/electron-api";

export type TorrentFile = ElectronTorrentFile;

export interface TorrentInstance extends ElectronTorrentInstance {
  files: TorrentFile[];
  streamUrl?: string;
}

export interface TorrentBackend {
  addMagnet(magnetLink: string): Promise<TorrentInstance>;
  addTorrentFile(bytes: Uint8Array): Promise<TorrentInstance>;
  getStats(): Promise<TorrentInstance | null>;
  clear(): Promise<void>;
  setMaxBufferMB(mb: number): void;
  probeAudioTracks(streamUrl: string): Promise<AudioTrackInfo[]>;
  probeSubtitles(streamUrl: string): Promise<SubtitleTrackInfo[]>;
  createAudioTrackStreamUrl(params: { streamUrl: string; trackIndex: number; startSeconds: number }): Promise<string | null>;
  createMultiplexedStreamUrl(params: { streamUrl: string; audioTrackIndex: number; startSeconds: number }): Promise<string | null>;
  createSubtitleStreamUrl(params: { streamUrl: string; trackIndex: number; startSeconds: number }): Promise<string | null>;
  destroy(): Promise<void>;
}

export class ElectronTorrentBackendAdapter implements TorrentBackend {
  private backend: ElectronTorrentBackend;

  constructor(backend: ElectronTorrentBackend) {
    this.backend = backend;
  }

  async addMagnet(magnetLink: string): Promise<TorrentInstance> {
    return this.backend.addMagnet(magnetLink) as Promise<TorrentInstance>;
  }

  async addTorrentFile(bytes: Uint8Array): Promise<TorrentInstance> {
    return this.backend.addTorrentFile(bytes) as Promise<TorrentInstance>;
  }

  async getStats(): Promise<TorrentInstance | null> {
    const stats = await this.backend.getStats();
    return stats ? (stats as unknown as TorrentInstance) : null;
  }

  async clear(): Promise<void> {
    await this.backend.clear();
  }

  setMaxBufferMB(mb: number): void {
    this.backend.setMaxBufferMB?.(mb);
  }

  async probeAudioTracks(streamUrl: string): Promise<AudioTrackInfo[]> {
    return this.backend.probeAudioTracks?.(streamUrl) ?? [];
  }

  async probeSubtitles(streamUrl: string): Promise<SubtitleTrackInfo[]> {
    return this.backend.probeSubtitles?.(streamUrl) ?? [];
  }

  async createAudioTrackStreamUrl(params: { streamUrl: string; trackIndex: number; startSeconds: number }): Promise<string | null> {
    return this.backend.createAudioTrackStreamUrl?.(params) ?? null;
  }

  async createMultiplexedStreamUrl(params: { streamUrl: string; audioTrackIndex: number; startSeconds: number }): Promise<string | null> {
    return this.backend.createMultiplexedStreamUrl?.(params) ?? null;
  }

  async createSubtitleStreamUrl(params: { streamUrl: string; trackIndex: number; startSeconds: number }): Promise<string | null> {
    return this.backend.createSubtitleStreamUrl?.(params) ?? null;
  }

  async destroy(): Promise<void> {
    await this.backend.clear();
  }
}

export function getElectronBackendAdapter(): ElectronTorrentBackendAdapter | null {
  if (typeof window === "undefined") return null;
  const backend = (window as unknown as ElectronWindow).torrsyncElectronTorrent ?? null;
  if (!backend) return null;
  return new ElectronTorrentBackendAdapter(backend);
}
