export type SyncAction = "play" | "pause" | "seek" | "state";

export interface AudioTrackInfo {
  index: number;
  label: string;
  language: string;
  codecName: string;
  channels: number | null;
  sampleRate: number | null;
}

export interface SubtitleTrackInfo {
  index: number;
  label: string;
  language: string;
  codecName: string;
  forced: boolean;
  default: boolean;
  streamUrl?: string;
}

export interface SyncMessage {
  action: SyncAction;
  position: number;
  server_ts: number;
  is_playing?: boolean;
  sourceKey?: string;
}

export type SharedTorrentSource =
  | {
      kind: "magnet";
      magnetLink: string;
      sourceKey: string;
    }
  | {
      kind: "file";
      fileName: string;
      bytes: Uint8Array;
      sourceKey: string;
    };

export interface RoomConfigMessage {
  syncToleranceSeconds: number;
}

export interface TorrentSourceMessage {
  source: SharedTorrentSource;
  selectedMediaIndex: number | null;
  selectedAudioTrackIndex: number | null;
  selectedSubtitleIndex: number | null;
}

export interface ChatMessage {
  id?: string;
  sender: string;
  text: string;
  timestamp: number;
}

export type ConnectionQuality = "good" | "fair" | "poor" | "unknown";

export type PeerRole = "master" | "slave";

export type PeerConnectionState = "connected" | "connecting" | "disconnected" | "error";

export interface Peer {
  id: string;
  name: string;
  role: PeerRole;
  connectionState: PeerConnectionState;
}
