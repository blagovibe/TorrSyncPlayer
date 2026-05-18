import { SYNC_CONFIG } from "../config";

export function clampSyncTolerance(value: number): number {
  if (!Number.isFinite(value) || value < 0) return SYNC_CONFIG.defaultToleranceSeconds;
  return value;
}
