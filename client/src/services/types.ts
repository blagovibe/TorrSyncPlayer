export type SyncAction = "play" | "pause" | "seek" | "state";

export interface SyncMessage {
  action: SyncAction;
  position: number;
  server_ts: number;
  is_playing?: boolean;
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
