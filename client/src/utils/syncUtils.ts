import { SYNC_CONFIG } from "../config";

export function clampSyncTolerance(value: number): number {
  if (!Number.isFinite(value) || value < 0) return SYNC_CONFIG.defaultToleranceSeconds;
  return value;
}

export function isPlaybackBlockedError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }

  const maybeDomException = error as { name?: string; message?: string };
  return (
    maybeDomException.name === "NotAllowedError" ||
    maybeDomException.message?.includes("play() failed because the user didn't interact") === true
  );
}
