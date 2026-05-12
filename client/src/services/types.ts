export type SyncAction = "play" | "pause" | "seek" | "state";

export interface AudioTrackInfo {
  index: number;
  label: string;
  language: string;
  codecName: string;
  channels: number | null;
  sampleRate: number | null;
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
      bytes: number[];
      sourceKey: string;
    };

export interface RoomConfigMessage {
  syncToleranceSeconds: number;
}
