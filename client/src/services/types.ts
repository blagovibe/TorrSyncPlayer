export type SyncAction = "play" | "pause" | "seek";

export interface SyncMessage {
  action: SyncAction;
  position: number;
  server_ts: number;
}
