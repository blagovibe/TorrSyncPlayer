import { useCallback, useEffect, useRef } from "react";
import SyncService from "../services/SyncService";
import { type SyncMessage } from "../services/types";
import { clampSyncTolerance } from "../utils/syncUtils";
import { useRoomStateContext } from "./useRoomStateContext";
import type P2PService from "../services/P2PService";
import type { TorrentService } from "./useTorrentLoader";
import { uiLogger } from "../utils/logger";

export interface SyncPlayback {
  syncServiceRef: React.MutableRefObject<SyncService | null>;
  seek: (timestamp: number) => void;
  setSyncTolerance: (value: number) => void;
  handleTimeUpdate: (currentTime: number, videoDuration: number, torrentService?: TorrentService | null) => void;
  tryApplyPendingRemoteSync: () => void;
}

export function useSyncPlayback(
  videoRef: React.RefObject<HTMLVideoElement | null>,
  p2pService: React.MutableRefObject<P2PService | null>,
  currentView: string,
  scheduleBroadcast: (targetPeerId?: string) => void,
): SyncPlayback {
  const roomState = useRoomStateContext();
  const {
    state: { peerRole, syncToleranceSeconds, selectedMediaFile, currentTorrentSource },
    setSyncToleranceSeconds, getCurrentSourceKey,
  } = roomState;

  const syncServiceRef = useRef<SyncService | null>(null);
  const isPlayerReadyRef = useRef(false);

  const disposeSync = useCallback(() => {
    syncServiceRef.current?.dispose();
    syncServiceRef.current = null;
  }, []);

  const tryApplyPendingRemoteSync = useCallback(() => {
    if (peerRole !== "slave") return;
    if (!isPlayerReadyRef.current || !selectedMediaFile || !syncServiceRef.current) return;
    const pending = roomState.state.pendingRemoteSync;
    if (!pending || !pending.sourceKey) return;
    const curKey = getCurrentSourceKey();
    if (curKey && pending.sourceKey !== curKey) return;
    roomState.setPendingSync(null);
    try {
      syncServiceRef.current.applyRemoteSync(pending);
    } catch (error) {
      uiLogger.error("Failed to apply remote sync:", error);
    }
  }, [peerRole, selectedMediaFile, roomState, getCurrentSourceKey]);

  useEffect(() => {
    disposeSync();
    if (currentView !== "room" || !videoRef.current || !peerRole || !p2pService.current) return;

    const transport = {
      sendSync: (msg: SyncMessage) => {
        if (!p2pService.current?.isConnected()) return;
        const key = getCurrentSourceKey();
        p2pService.current.sendSync(key && msg.sourceKey !== key ? { ...msg, sourceKey: key } : msg);
      },
    };

    const svc = new SyncService(transport, videoRef.current, peerRole, syncToleranceSeconds);
    syncServiceRef.current = svc;
    tryApplyPendingRemoteSync();
    if (peerRole === "master" && currentTorrentSource) scheduleBroadcast();

    return () => { svc.dispose(); if (syncServiceRef.current === svc) syncServiceRef.current = null; };
  }, [currentView, peerRole, syncToleranceSeconds, videoRef, p2pService, getCurrentSourceKey, currentTorrentSource, tryApplyPendingRemoteSync, scheduleBroadcast, disposeSync]);

  useEffect(() => {
    syncServiceRef.current?.setSyncToleranceSeconds(syncToleranceSeconds);
  }, [syncToleranceSeconds]);

  const seek = useCallback((timestamp: number) => {
    if (peerRole === "master" && syncServiceRef.current) {
      try {
        syncServiceRef.current.seek(timestamp);
      } catch (error) {
        uiLogger.error("Seek failed:", error);
      }
    }
  }, [peerRole]);

  const setSyncTolerance = useCallback((value: number) => {
    const t = clampSyncTolerance(value);
    setSyncToleranceSeconds(t);
    try {
      syncServiceRef.current?.setSyncToleranceSeconds(t);
    } catch (error) {
      uiLogger.error("Failed to set sync tolerance:", error);
    }
  }, [setSyncToleranceSeconds]);

  const handleTimeUpdate = useCallback((currentTime: number, videoDuration: number, torrentService?: TorrentService | null) => {
    if (!selectedMediaFile?.file || !torrentService) return;
    if (!videoDuration || videoDuration <= 0 || !Number.isFinite(videoDuration)) return;
    const fileLength = selectedMediaFile.file.length;
    if (fileLength !== undefined) {
      try {
        torrentService.updatePlaybackPosition(currentTime, fileLength, videoDuration);
      } catch (error) {
        uiLogger.warn("Failed to update playback position:", error);
      }
    }
  }, [selectedMediaFile]);

  return { syncServiceRef, seek, setSyncTolerance, handleTimeUpdate, tryApplyPendingRemoteSync };
}
