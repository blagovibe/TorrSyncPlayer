import { useCallback, useEffect, useRef, useState } from "react";
import HomePage from "./components/HomePage";
import RoomPage from "./components/RoomPage";
import P2PService from "./services/P2PService";
import SyncService from "./services/SyncService";
import TorrentService, { type TorrentMediaFile } from "./services/TorrentService";
import { type ConnectionQuality, type Peer, type PeerConnectionState, type PeerRole, type SharedTorrentSource, type SyncMessage } from "./services/types";
import { clampSyncTolerance, isPlaybackBlockedError } from "./utils/syncUtils";
import { createMagnetSource, createTorrentFileSource } from "./utils/torrent";
import { formatSpeed } from "./utils/format";
import { p2pLogger, uiLogger } from "./utils/logger";
import { SYNC_CONFIG } from "./config";
import ConfirmModal from "./components/ConfirmModal";
import { useRoomStateContext } from "./hooks/useRoomStateContext";

import "./App.css";

export type View = "home" | "room";

type TorrentLoadRequest = {
  source: SharedTorrentSource;
  selectedMediaIndex: number | null;
  selectedAudioTrackIndex: number | null;
  selectedSubtitleIndex: number | null;
  autoplay: boolean;
  broadcast: boolean;
};

function App() {
  const [currentView, setCurrentView] = useState<View>("home");
  const [peerId, setPeerId] = useState("");
  const [peerRole, setPeerRole] = useState<PeerRole | null>(null);
  const [peers, setPeers] = useState<Peer[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [magnetLink, setMagnetLink] = useState("");
  const [torrentFile, setTorrentFile] = useState<File | null>(null);
  const [isLoadingTorrent, setIsLoadingTorrent] = useState(false);
  const [torrentProgress, setTorrentProgress] = useState(0);
  const [downloadSpeed, setDownloadSpeed] = useState("0 B/s");
  const [torrentError, setTorrentError] = useState<string | null>(null);
  const [mediaFiles, setMediaFiles] = useState<TorrentMediaFile[]>([]);

  const [torrentPeerCount, setTorrentPeerCount] = useState(0);
  const [trackerLost, setTrackerLost] = useState(false);
  const [playbackNotice, setPlaybackNotice] = useState<string | null>(null);
  const [syncToleranceSeconds, setSyncToleranceSeconds] = useState(SYNC_CONFIG.defaultToleranceSeconds);
  const [isPlayerReady, setIsPlayerReady] = useState(false);

  const [bufferWindowMB, setBufferWindowMB] = useState(50);
  const [maxBufferMB, setMaxBufferMB] = useState(500);
  const [connectionQuality, setConnectionQuality] = useState<ConnectionQuality>("unknown");
  const [rttMs, setRttMs] = useState<number | null>(null);
  const [showLeaveConfirm, setShowLeaveConfirm] = useState(false);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [showCloseConfirm, setShowCloseConfirm] = useState(false);
  const [chatMessages, setChatMessages] = useState<{ id?: string; sender: string; text: string; timestamp: number }[]>([]);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const p2pServiceRef = useRef<P2PService | null>(null);
  const torrentServiceRef = useRef<TorrentService | null>(null);
  const [torrentServiceVersion, setTorrentServiceVersion] = useState(0);
  const getTorrentService = useCallback(() => {
    if (!torrentServiceRef.current || (torrentServiceRef.current as unknown as { isDestroyed: boolean }).isDestroyed) {
      torrentServiceRef.current = new TorrentService();
    }
    return torrentServiceRef.current;
  }, []);
  const syncServiceRef = useRef<SyncService | null>(null);

  const roomState = useRoomStateContext();
  const {
    state: {
      currentTorrentSource,
      selectedMediaIndex,
      selectedMediaFile,
      selectedMediaLabel,
      selectedMediaKind,
      selectedMediaAudioTracks,
      selectedSubtitles,
      selectedAudioTrackIndex,
      selectedSubtitleIndex,
      pendingRemoteSync,
      peerRole: ctxPeerRole,
    },
    setTorrentSource,
    setMediaIndex,
    setMediaFile,
    setMediaLabel,
    setMediaKind,
    setMediaAudioTracks,
    setSubtitles: setCtxSubtitles,
    setAudioTrackIndex,
    setSubtitleIndex,
    setPendingSync,
    setPeerRole: setCtxPeerRole,
    getCurrentSourceKey,
  } = roomState;

  const pendingTorrentLoadRef = useRef<TorrentLoadRequest | null>(null);
  const isProcessingTorrentLoadRef = useRef(false);
  const isLoadingTorrentRef = useRef(false);
  const isPlayerReadyRef = useRef(false);
  const torrentLoadAbortRef = useRef<AbortController | null>(null);
  const torrentLoadVersionRef = useRef(0);

  const disposeSyncService = () => {
    syncServiceRef.current?.dispose();
    syncServiceRef.current = null;
  };

  const resetTorrentState = () => {
    setIsLoadingTorrent(false);
    setTorrentProgress(0);
    setDownloadSpeed("0 B/s");
    setTorrentError(null);
    setTorrentFile(null);
    setMediaFiles([]);
    setMediaIndex(null);
    setMediaLabel(null);
    setMediaKind(null);
    setMediaFile(null);
    setMediaAudioTracks([]);
    setAudioTrackIndex(null);
    setSubtitleIndex(null);
    setCtxSubtitles([]);
    setTorrentPeerCount(0);
    setPlaybackNotice(null);
    setTorrentSource(null);
    setPendingSync(null);
    pendingTorrentLoadRef.current = null;
    isProcessingTorrentLoadRef.current = false;
    isLoadingTorrentRef.current = false;
  };

  const setSelectedAudioTrackSelection = useCallback((trackIndex: number | null) => {
    setAudioTrackIndex(trackIndex);
  }, [setAudioTrackIndex]);

  const setSelectedSubtitleSelection = useCallback((trackIndex: number | null) => {
    setSubtitleIndex(trackIndex);
  }, [setSubtitleIndex]);

  const enrichSyncMessage = useCallback((message: SyncMessage): SyncMessage => {
    const sourceKey = getCurrentSourceKey();
    if (!sourceKey || message.sourceKey === sourceKey) {
      return message;
    }
    return { ...message, sourceKey };
  }, [getCurrentSourceKey]);

  const broadcastTimeoutRef = useRef<number | null>(null);

  const clearBroadcastTimeout = () => {
    if (broadcastTimeoutRef.current !== null) {
      window.clearTimeout(broadcastTimeoutRef.current);
      broadcastTimeoutRef.current = null;
    }
  };

  const broadcastCurrentRoomState = useCallback((targetPeerId?: string) => {
    if (!p2pServiceRef.current?.isHost()) return;
    const p2pService = p2pServiceRef.current;
    if (!p2pService) return;
    if (currentTorrentSource) {
      p2pService.sendTorrentSource(
        currentTorrentSource,
        selectedMediaIndex,
        selectedAudioTrackIndex,
        selectedSubtitleIndex,
        targetPeerId,
      );
    }
    p2pService.sendRoomConfig(syncToleranceSeconds, targetPeerId);
    const playbackSnapshot = syncServiceRef.current?.createSnapshot();
    if (playbackSnapshot) {
      p2pService.sendSync(enrichSyncMessage(playbackSnapshot), targetPeerId);
    }
  }, [currentTorrentSource, selectedMediaIndex, selectedAudioTrackIndex, selectedSubtitleIndex, syncToleranceSeconds, enrichSyncMessage]);

  const scheduleBroadcast = useCallback((targetPeerId?: string) => {
    clearBroadcastTimeout();
    broadcastTimeoutRef.current = window.setTimeout(() => {
      broadcastCurrentRoomState(targetPeerId);
      broadcastTimeoutRef.current = null;
    }, 500);
  }, [broadcastCurrentRoomState]);

  const tryApplyPendingRemoteSync = useCallback(() => {
    if (peerRole !== "slave") return;
    if (
      !isPlayerReadyRef.current ||
      isLoadingTorrentRef.current ||
      !selectedMediaFile ||
      !syncServiceRef.current
    ) return;
    const pendingSync = pendingRemoteSync;
    if (!pendingSync) return;
    if (!pendingSync.sourceKey) return;
    const currentSourceKey = getCurrentSourceKey();
    if (currentSourceKey && pendingSync.sourceKey !== currentSourceKey) return;
    setPendingSync(null);
    syncServiceRef.current.applyRemoteSync(pendingSync);
  }, [peerRole, selectedMediaFile, pendingRemoteSync, getCurrentSourceKey, setPendingSync]);

  const playMediaFile = useCallback(async (mediaFile: TorrentMediaFile, autoplay = true) => {
    const mediaElement = videoRef.current;
    if (!mediaElement) throw new Error("Media player is not ready");

    await getTorrentService().streamToMedia(mediaFile.file, mediaElement);
    setMediaFile(mediaFile);
    setMediaAudioTracks([]);
    setCtxSubtitles([]);
    void getTorrentService()
      .probeAudioTracks(mediaFile.file)
      .then((audioTracks) => {
        if (selectedMediaFile?.index === mediaFile.index) {
          setMediaAudioTracks(audioTracks);
        }
      })
      .catch(() => undefined);
    void getTorrentService()
      .probeSubtitles(mediaFile.file)
      .then((subtitles) => {
        if (selectedMediaFile?.index === mediaFile.index) {
          setCtxSubtitles(subtitles);
        }
      })
      .catch(() => undefined);
    mediaElement.defaultMuted = false;
    mediaElement.muted = false;
    if (mediaElement.volume <= 0) {
      mediaElement.volume = 1;
    }
    setMediaIndex(mediaFile.index);
    setMediaLabel(mediaFile.name);
    setMediaKind(mediaFile.kind);

    if (!autoplay) return;
    try {
      await mediaElement.play();
      setPlaybackNotice(null);
    } catch (error) {
      if (isPlaybackBlockedError(error)) {
        setPlaybackNotice("Autoplay was blocked. Press Play in the player to start the movie.");
      } else {
        throw error;
      }
    }
  }, [getTorrentService, setMediaFile, selectedMediaFile, setMediaIndex, setMediaLabel, setMediaKind, setMediaAudioTracks, setCtxSubtitles]);

  const loadTorrentFile = async (file: File) => {
    if (peerRole !== "master") return;
    const MAX_TORRENT_FILE_SIZE = 10 * 1024 * 1024;
    if (file.size > MAX_TORRENT_FILE_SIZE) {
      setTorrentError(`Torrent file too large (${(file.size / 1024 / 1024).toFixed(1)} MB). Maximum size is ${MAX_TORRENT_FILE_SIZE / 1024 / 1024} MB.`);
      return;
    }
    try {
      const torrentBytes = new Uint8Array(await file.arrayBuffer());
      setMagnetLink("");
      requestTorrentLoad({
        source: createTorrentFileSource(file.name, torrentBytes),
        selectedMediaIndex: null,
        selectedAudioTrackIndex: null,
        selectedSubtitleIndex: null,
        autoplay: true,
        broadcast: true,
      });
    } catch (error) {
      uiLogger.error("Torrent file load failed:", error);
    }
  };

  const loadTorrentRequest = useCallback(async (request: TorrentLoadRequest) => {
    if (!videoRef.current) throw new Error("Media player is not ready");
    if (torrentLoadAbortRef.current?.signal.aborted) throw new Error("Torrent load cancelled");
    const requestVersion = torrentLoadVersionRef.current;

    const currentSource = currentTorrentSource;
    const currentSelectedIndex = selectedMediaIndex;
    const currentSelectedAudioTrackIndex = selectedAudioTrackIndex;
    const currentSelectedSubtitleIndex = selectedSubtitleIndex;

    if (currentSource?.sourceKey === request.source.sourceKey) {
      const desiredIndex = request.selectedMediaIndex !== null ? request.selectedMediaIndex : currentSelectedIndex;
      const desiredAudioTrackIndex = request.selectedAudioTrackIndex ?? currentSelectedAudioTrackIndex;
      const desiredSubtitleIndex = request.selectedSubtitleIndex ?? currentSelectedSubtitleIndex;
      const currentMediaFile =
        selectedMediaFile ??
        (currentSelectedIndex !== null
          ? mediaFiles.find((file) => file.index === currentSelectedIndex) ?? null
          : null);

      if (desiredAudioTrackIndex !== currentSelectedAudioTrackIndex) {
        setSelectedAudioTrackSelection(desiredAudioTrackIndex);
      }
      if (desiredSubtitleIndex !== currentSelectedSubtitleIndex) {
        setSelectedSubtitleSelection(desiredSubtitleIndex);
      }
      if (desiredIndex !== null && desiredIndex !== currentSelectedIndex) {
        const nextMediaFile = mediaFiles.find((file) => file.index === desiredIndex);
        if (!nextMediaFile) throw new Error("Requested media file is not available in the current torrent");
        await playMediaFile(nextMediaFile, request.autoplay);
      } else if (request.autoplay && currentMediaFile && videoRef.current?.paused) {
        await playMediaFile(currentMediaFile, true);
      }
      if (request.broadcast && peerRole === "master") scheduleBroadcast();
      tryApplyPendingRemoteSync();
      return;
    }

    setMediaFiles([]);
    setMediaIndex(null);
    setMediaLabel(null);
    setMediaKind(null);
    setMediaFile(null);
    setMediaAudioTracks([]);
    setSelectedAudioTrackSelection(request.selectedAudioTrackIndex);

    const torrent =
      request.source.kind === "magnet"
        ? await getTorrentService().addMagnet(request.source.magnetLink)
        : await getTorrentService().addTorrentFile(new Uint8Array(request.source.bytes));

    if (torrentLoadAbortRef.current?.signal.aborted) return;
    if (torrentLoadVersionRef.current !== requestVersion) return;

    const playableMediaFiles = getTorrentService().getPlayableMediaFiles(torrent);
    setMediaFiles(playableMediaFiles);

    if (playableMediaFiles.length === 0) throw new Error("No supported video or audio file found in torrent");

    const preferredMediaFile =
      request.selectedMediaIndex !== null
        ? playableMediaFiles.find((file) => file.index === request.selectedMediaIndex) ??
          getTorrentService().getPreferredMediaFile(torrent)
        : getTorrentService().getPreferredMediaFile(torrent);

    await playMediaFile(preferredMediaFile, request.autoplay);
    setTorrentSource(request.source);

    if (request.broadcast && peerRole === "master") scheduleBroadcast();
    tryApplyPendingRemoteSync();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentTorrentSource, selectedMediaIndex, selectedAudioTrackIndex, selectedSubtitleIndex, selectedMediaFile, mediaFiles, peerRole, getTorrentService, playMediaFile, scheduleBroadcast, tryApplyPendingRemoteSync, setSelectedAudioTrackSelection, setSelectedSubtitleSelection, setMediaIndex, setMediaFile, setTorrentSource]);

  const processTorrentLoadQueueRef = useRef<() => Promise<void>>(async () => {});

  const processTorrentLoadQueue = useCallback(async () => {
    if (isProcessingTorrentLoadRef.current) return;
    const nextRequest = pendingTorrentLoadRef.current;
    if (!nextRequest) return;
    if (!isPlayerReadyRef.current || !videoRef.current) return;

    pendingTorrentLoadRef.current = null;
    isProcessingTorrentLoadRef.current = true;
    isLoadingTorrentRef.current = true;
    setIsLoadingTorrent(true);
    setTorrentError(null);
    setTorrentProgress(0);
    setDownloadSpeed("0 B/s");
    setTorrentPeerCount(0);
    setPlaybackNotice(null);

    torrentLoadVersionRef.current++;
    torrentLoadAbortRef.current?.abort();
    const abortController = new AbortController();
    torrentLoadAbortRef.current = abortController;

    const loadVersion = torrentLoadVersionRef.current;
    try {
      await loadTorrentRequest(nextRequest);
    } catch (error) {
      if (abortController.signal.aborted) return;
      const message = error instanceof Error ? error.message : "Unable to load torrent";
      setTorrentError(message);
      uiLogger.error("Torrent load failed:", error);
    } finally {
      if (torrentLoadAbortRef.current === abortController) {
        isLoadingTorrentRef.current = false;
        isProcessingTorrentLoadRef.current = false;
        torrentLoadAbortRef.current = null;
      }
      setIsLoadingTorrent(false);
      if (pendingTorrentLoadRef.current && !abortController.signal.aborted && torrentLoadVersionRef.current === loadVersion) {
        const pending = pendingTorrentLoadRef.current;
        void new Promise<void>((resolve) => setTimeout(resolve, 0)).then(() => {
          if (pendingTorrentLoadRef.current === pending && !torrentLoadAbortRef.current?.signal.aborted) {
            void processTorrentLoadQueueRef.current();
          }
        });
      }
    }
  }, [loadTorrentRequest]);

  processTorrentLoadQueueRef.current = processTorrentLoadQueue;

  const requestTorrentLoad = useCallback((request: TorrentLoadRequest) => {
    pendingTorrentLoadRef.current = request;
    if (currentView !== "room" || !isPlayerReadyRef.current || !videoRef.current) return;
    if (!isProcessingTorrentLoadRef.current) {
      void processTorrentLoadQueueRef.current();
    }
  }, [currentView]);

  const initializeP2PService = async (role: "host" | "guest") => {
    p2pServiceRef.current?.disconnect();
    disposeSyncService();
    setPendingSync(null);

    const p2pService = new P2PService();
    if (role === "host") {
      p2pService.setHost();
    } else {
      p2pService.setGuest();
    }
    p2pServiceRef.current = p2pService;

    p2pService.on("connected", () => {
      setIsConnected(true);
      setIsConnecting(false);
      setPeers((prev) =>
        prev.map((peer) =>
          peer.id !== "self" ? { ...peer, connectionState: "connected" as PeerConnectionState } : peer,
        ),
      );
    });

    p2pService.on("disconnected", () => {
      setIsConnected(false);
      setPeers((prev) =>
        prev.map((peer) => ({ ...peer, connectionState: "disconnected" as PeerConnectionState })),
      );
      setPendingSync(null);
    });

    p2pService.on("peer_connected", (connectedPeerId) => {
      setPeers((prev) => {
        if (prev.some((peer) => peer.id === connectedPeerId)) return prev;
        return [...prev, { id: connectedPeerId, name: "Peer", role: "slave", connectionState: "connected" }];
      });
      if (role === "host") scheduleBroadcast(connectedPeerId);
    });

    p2pService.on("peer_disconnected", (disconnectedPeerId) => {
      setPeers((prev) => prev.filter((peer) => peer.id !== disconnectedPeerId));
      p2pService.clearRateLimitForPeer(disconnectedPeerId);
    });

    p2pService.on("sync", (message) => {
      if (ctxPeerRole !== "slave") return;
      setPendingSync(message);
      if (isPlayerReadyRef.current && syncServiceRef.current && selectedMediaFile) {
        setPendingSync(null);
        syncServiceRef.current.applyRemoteSync(message);
      }
    });

    p2pService.on("torrent_source", (message) => {
      if (ctxPeerRole !== "slave") return;
      if (
        pendingRemoteSync &&
        (!pendingRemoteSync.sourceKey || pendingRemoteSync.sourceKey !== message.source.sourceKey)
      ) {
        setPendingSync(null);
      }
      requestTorrentLoad({
        source: message.source,
        selectedMediaIndex: message.selectedMediaIndex,
        selectedAudioTrackIndex: message.selectedAudioTrackIndex,
        selectedSubtitleIndex: message.selectedSubtitleIndex,
        autoplay: true,
        broadcast: false,
      });
    });

    p2pService.on("room_config", (message) => {
      const nextTolerance = clampSyncTolerance(message.syncToleranceSeconds);
      setSyncToleranceSeconds(nextTolerance);
      syncServiceRef.current?.setSyncToleranceSeconds(nextTolerance);
    });

    p2pService.on("reconnecting", (attempt, delayMs) => {
      p2pLogger.info(`Reconnecting attempt ${attempt}, delay ${delayMs}ms`);
      setConnectionError(`Connection lost. Reconnecting in ${Math.round(delayMs / 1000)}s... (attempt ${attempt})`);
    });

    p2pService.on("error", (error) => {
      p2pLogger.error("P2P error:", error);
      setConnectionError(error.message);
      setIsConnecting(false);
    });

    p2pService.on("connection_quality", (quality) => {
      setConnectionQuality(quality);
      setRttMs(p2pService.getLastRttMs());
    });

    p2pService.on("chat_received", (senderId, content) => {
      const message = { id: `${senderId}-${Date.now()}`, sender: senderId, text: content, timestamp: Date.now() };
      setChatMessages(prev => [...prev, message]);
    });

    await p2pService.initialize();
    setPeerId(p2pService.getPeerId());
    return p2pService;
  };

  const handleCreateRoom = async () => {
    if (isConnecting) return;
    setIsConnecting(true);
    setConnectionError(null);
    try {
      await initializeP2PService("host");
      setPeerRole("master");
      setCtxPeerRole("master");
      setPeers([{ id: "self", name: "You", role: "master", connectionState: "connected" }]);
      setCurrentView("room");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unable to initialize room";
      try { p2pServiceRef.current?.disconnect(); } catch { /* non-fatal */ }
      p2pServiceRef.current = null;
      setConnectionError(message);
      setPeerRole(null);
      setCtxPeerRole(null);
      setPeers([]);
      setIsConnected(false);
    } finally {
      setIsConnecting(false);
    }
  };

  const handleConnectToPeer = async (targetPeerId: string) => {
    if (isConnecting) return;
    const normalizedId = targetPeerId.trim().toUpperCase();
    if (!normalizedId || normalizedId.length !== 6) {
      setConnectionError("Invalid peer ID");
      return;
    }
    setIsConnecting(true);
    setConnectionError(null);
    try {
      const p2pService = await initializeP2PService("guest");
      await p2pService.connect(`torrsync-${normalizedId}`);
      setPeerRole("slave");
      setCtxPeerRole("slave");
      setIsConnected(true);
      setPeers([
        { id: "self", name: "You", role: "slave", connectionState: "connected" },
        { id: normalizedId, name: "Host", role: "master", connectionState: "connected" },
      ]);
      setCurrentView("room");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Connection failed";
      try { p2pServiceRef.current?.disconnect(); } catch { /* non-fatal */ }
      p2pServiceRef.current = null;
      setConnectionError(message);
      setPeerRole(null);
      setCtxPeerRole(null);
      setPeers([]);
      setIsConnected(false);
    } finally {
      setIsConnecting(false);
    }
  };

  const handleJoinRoom = (code: string) => {
    handleConnectToPeer(code);
  };

  const handleLeaveRoom = async () => {
    clearBroadcastTimeout();
    torrentLoadAbortRef.current?.abort();
    torrentLoadAbortRef.current = null;
    if (p2pServiceRef.current) {
      p2pServiceRef.current.disconnect();
      p2pServiceRef.current = null;
    }
    disposeSyncService();
    const torrentService = torrentServiceRef.current ?? getTorrentService();
    await torrentService.destroy().catch(() => undefined);
    torrentServiceRef.current = null;
    const video = videoRef.current;
    video?.pause();
    if (video) {
      video.removeAttribute("src");
      video.load();
    }
    setCurrentView("home");
    setPeerId("");
    setPeerRole(null);
    setCtxPeerRole(null);
    setPeers([]);
    setIsConnected(false);
    setIsConnecting(false);
    setIsPlayerReady(false);
    isPlayerReadyRef.current = false;
    setConnectionError(null);
    setPlaybackNotice(null);
    setPendingSync(null);
    setTrackerLost(false);
    setShowLeaveConfirm(false);
    resetTorrentState();
    roomState.reset();
    setChatMessages([]);
  };

  const handleResetTorrentInRoom = async () => {
    if (peerRole !== "master") return;
    torrentLoadAbortRef.current?.abort();
    torrentLoadAbortRef.current = null;
    const oldService = torrentServiceRef.current;
    torrentServiceRef.current = null;
    setMediaFile(null);
    setTorrentSource(null);
    resetTorrentState();
    setMagnetLink("");
    setTorrentFile(null);
    if (oldService) {
      await oldService.destroy().catch(() => undefined);
    }
    setTorrentServiceVersion((v) => v + 1);
    setShowResetConfirm(false);
  };

  const handleBufferingChange = useCallback((isBuffering: boolean) => {
    if (isBuffering) {
      setPlaybackNotice("Buffering...");
    } else {
      setPlaybackNotice((prev) => (prev === "Buffering..." ? null : prev));
    }
  }, []);

  const handleLoadMagnet = async () => {
    if (peerRole !== "master" || !magnetLink.trim()) return;
    setTorrentFile(null);
    requestTorrentLoad({
      source: createMagnetSource(magnetLink),
      selectedMediaIndex: null,
      selectedAudioTrackIndex: null,
      selectedSubtitleIndex: null,
      autoplay: true,
      broadcast: true,
    });
  };

  const handleTorrentFileChange = (file: File | null) => {
    setTorrentFile(file);
    if (!file) return;
    void loadTorrentFile(file);
  };

  const handleLoadTorrentFile = async () => {
    if (!torrentFile) return;
    await loadTorrentFile(torrentFile);
  };

  const handleSelectMediaFile = useCallback((mediaFile: TorrentMediaFile) => {
    if (peerRole !== "master" || !currentTorrentSource) return;
    requestTorrentLoad({
      source: currentTorrentSource,
      selectedMediaIndex: mediaFile.index,
      selectedAudioTrackIndex: null,
      selectedSubtitleIndex: null,
      autoplay: true,
      broadcast: true,
    });
    setSelectedAudioTrackSelection(null);
  }, [peerRole, currentTorrentSource, requestTorrentLoad, setSelectedAudioTrackSelection]);

  const handleMuxStreamRequest = useCallback(
    async (startSeconds: number): Promise<string | null> => {
      const mediaFile = selectedMediaFile;
      if (!mediaFile) return null;
      return getTorrentService().createMuxStreamUrl(
        mediaFile.file,
        selectedAudioTrackIndex,
        startSeconds,
      );
    },
    [getTorrentService, selectedMediaFile, selectedAudioTrackIndex],
  );

  const handleAudioTrackChange = useCallback((trackIndex: number | null) => {
    setSelectedAudioTrackSelection(trackIndex);
    if (
      peerRole !== "master" ||
      isLoadingTorrentRef.current ||
      !currentTorrentSource ||
      !p2pServiceRef.current?.isHost() ||
      !p2pServiceRef.current.isConnected()
    ) return;
    broadcastCurrentRoomState();
  }, [peerRole, currentTorrentSource, setSelectedAudioTrackSelection, broadcastCurrentRoomState]);

  const handleSubtitleTrackChange = useCallback((trackIndex: number | null) => {
    setSelectedSubtitleSelection(trackIndex);
    if (
      peerRole !== "master" ||
      isLoadingTorrentRef.current ||
      !currentTorrentSource ||
      !p2pServiceRef.current?.isHost() ||
      !p2pServiceRef.current.isConnected()
    ) return;
    broadcastCurrentRoomState();
  }, [peerRole, currentTorrentSource, setSelectedSubtitleSelection, broadcastCurrentRoomState]);

  const handleTimeUpdate = useCallback(
    (currentTime: number, videoDuration: number) => {
      const mediaFile = selectedMediaFile;
      if (!mediaFile) return;
      getTorrentService().updatePlaybackPosition(currentTime, mediaFile.length, videoDuration);
    },
    [getTorrentService, selectedMediaFile],
  );

  const handleSyncToleranceChange = (value: number) => {
    const nextTolerance = clampSyncTolerance(value);
    setSyncToleranceSeconds(nextTolerance);
    syncServiceRef.current?.setSyncToleranceSeconds(nextTolerance);
    if (p2pServiceRef.current?.isHost() && p2pServiceRef.current.isConnected()) {
      p2pServiceRef.current.sendRoomConfig(nextTolerance);
    }
  };

  const handleBufferSettingsChange = useCallback(
    (bufferWindowMB: number, maxBufferMB: number) => {
      getTorrentService().setBufferSettings(bufferWindowMB, maxBufferMB);
      setBufferWindowMB(bufferWindowMB);
      setMaxBufferMB(maxBufferMB);
    },
    [getTorrentService],
  );

  const handleSeek = useCallback(
    (timestamp: number) => {
      if (peerRole === "master" && syncServiceRef.current) {
        syncServiceRef.current.seek(timestamp);
      }
      getTorrentService().prioritizeNow();
    },
    [peerRole, getTorrentService],
  );

  const currentMediaFile = selectedMediaFile;
  const selectedMediaBufferProgress = Math.round(
    ((currentMediaFile?.file.progress != null && currentMediaFile.file.progress > 0)
      ? currentMediaFile.file.progress
      : torrentProgress / 100) * 100,
  );

  const torrentPeerHint =
    getTorrentService().isElectronBackendEnabled()
      ? torrentPeerCount > 0
        ? `${torrentPeerCount} public peer${torrentPeerCount === 1 ? "" : "s"} discovered via tracker, DHT, and PEX`
        : "Looking for public peers via tracker, DHT, and PEX"
      : torrentPeerCount > 0
        ? `${torrentPeerCount} public WebRTC peer${torrentPeerCount === 1 ? "" : "s"} discovered via trackers`
        : "Looking for public WebRTC peers";

  const bufferHint = currentMediaFile
    ? selectedMediaBufferProgress >= 100
      ? "Selected file is fully buffered."
      : selectedMediaBufferProgress > 0
        ? "Buffering — loading data around current position..."
        : "Selected file is buffering from the swarm."
    : "Load a torrent and pick a movie to see file buffering progress.";

  const sharedTorrentLabel = currentTorrentSource
    ? currentTorrentSource.kind === "magnet"
      ? "Magnet link"
      : currentTorrentSource.fileName
    : torrentFile?.name ?? null;

  const trackerLostRef = useRef(trackerLost);
  useEffect(() => {
    trackerLostRef.current = trackerLost;
  }, [trackerLost]);

  useEffect(() => {
    const torrentService = getTorrentService();
    const offTorrentError = torrentService.on("error", (error) => {
      uiLogger.error("Torrent error", error);
      setTorrentError(error.message);
      setIsLoadingTorrent(false);
      isLoadingTorrentRef.current = false;
    });
    const offTorrentProgress = torrentService.on("progress", (progress) => {
      setTorrentProgress(Math.round(progress * 100));
    });
    const offTorrentSpeed = torrentService.on("speed", (speed) => {
      setDownloadSpeed(formatSpeed(speed));
    });
    const offTorrentPeerCount = torrentService.on("peerCount", (peerCount) => {
      setTorrentPeerCount(peerCount);
      if (peerCount > 0) {
        setTrackerLost(false);
      } else if (selectedMediaFile && !isLoadingTorrentRef.current && currentTorrentSource) {
        setTrackerLost(true);
      }
    });
    return () => {
      offTorrentError();
      offTorrentProgress();
      offTorrentSpeed();
      offTorrentPeerCount();
    };
  }, [getTorrentService, torrentServiceVersion, selectedMediaFile, currentTorrentSource]);

  useEffect(() => {
    syncServiceRef.current?.setSyncToleranceSeconds(syncToleranceSeconds);
  }, [syncToleranceSeconds]);

  useEffect(() => {
    disposeSyncService();
    if (currentView !== "room" || !videoRef.current || !peerRole || !p2pServiceRef.current) return;

    const p2pSyncTransport = {
      sendSync: (message: SyncMessage) => {
        if (p2pServiceRef.current?.isConnected()) {
          p2pServiceRef.current.sendSync(enrichSyncMessage(message));
        }
      },
    };

    const syncService = new SyncService(
      p2pSyncTransport,
      videoRef.current,
      peerRole,
      syncToleranceSeconds,
    );
    syncServiceRef.current = syncService;
    tryApplyPendingRemoteSync();
    if (peerRole === "master" && currentTorrentSource) {
      broadcastCurrentRoomState();
    }
    return () => {
      syncService.dispose();
      if (syncServiceRef.current === syncService) {
        syncServiceRef.current = null;
      }
    };
  }, [currentView, peerRole, torrentServiceVersion, enrichSyncMessage, syncToleranceSeconds, tryApplyPendingRemoteSync, broadcastCurrentRoomState, currentTorrentSource]);

  const isCleanedUpRef = useRef(false);
  useEffect(() => {
    const doCleanup = () => {
      if (isCleanedUpRef.current) return;
      isCleanedUpRef.current = true;
      p2pServiceRef.current?.disconnect();
      p2pServiceRef.current = null;
      disposeSyncService();
      torrentServiceRef.current?.destroy().catch(() => undefined);
      torrentServiceRef.current = null;
      const video = videoRef.current;
      video?.pause();
      video?.removeAttribute("src");
      video?.load();
    };
    const onBeforeUnload = () => { doCleanup(); };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => {
      window.removeEventListener("beforeunload", onBeforeUnload);
      doCleanup();
    };
  }, []);

  useEffect(() => {
    if (
      currentView !== "room" ||
      !isPlayerReady ||
      !videoRef.current ||
      isProcessingTorrentLoadRef.current
    ) return;
    if (pendingTorrentLoadRef.current) {
      void processTorrentLoadQueueRef.current();
    }
    tryApplyPendingRemoteSync();
  }, [currentView, isPlayerReady, tryApplyPendingRemoteSync]);

  useEffect(() => {
    const win = window as Window & {
      torrsyncElectronWindow?: {
        onCloseRequest: (callback: () => void) => void;
        closeConfirmed: () => void;
        closeCancelled: () => void;
      };
    };
    if (!win.torrsyncElectronWindow) return;
    const electronWindow = win.torrsyncElectronWindow;
    if (!electronWindow) return;
    const { onCloseRequest } = electronWindow;
    onCloseRequest(() => {
      setShowCloseConfirm(true);
    });
    return () => {};
  }, []);

  const handleCloseConfirmed = useCallback(() => {
    setShowCloseConfirm(false);
    const win = window as Window & { torrsyncElectronWindow?: { closeConfirmed: () => void } };
    win.torrsyncElectronWindow?.closeConfirmed();
  }, []);

  const handleCloseCancelled = useCallback(() => {
    setShowCloseConfirm(false);
    const win = window as Window & { torrsyncElectronWindow?: { closeCancelled: () => void } };
    win.torrsyncElectronWindow?.closeCancelled();
  }, []);

  const MAX_CHAT_MESSAGE_LENGTH = 500;
  const chatRateTimestamps = useRef<number[]>([]);
  const chatRateLimitPer10s = 10;

  const isChatRateLimited = useCallback(() => {
    const now = Date.now();
    const recent = chatRateTimestamps.current.filter(ts => now - ts < 10_000);
    chatRateTimestamps.current = recent;
    if (recent.length >= chatRateLimitPer10s) return true;
    recent.push(now);
    return false;
  }, []);

  const handleSendChat = useCallback((text: string) => {
    if (!text.trim() || !p2pServiceRef.current) return;
    const trimmed = text.trim();
    if (trimmed.length > MAX_CHAT_MESSAGE_LENGTH) return;
    if (isChatRateLimited()) return;
    const message = { id: `${Date.now()}-${peerId}`, sender: peerId, text: trimmed, timestamp: Date.now() };
    setChatMessages(prev => [...prev, message]);
    p2pServiceRef.current.sendChat(message.text);
  }, [peerId, isChatRateLimited]);

  return (
    <main className="app-shell">
      {currentView === "home" ? (
         <HomePage
           peerId={peerId}
           onCreateRoom={handleCreateRoom}
           onJoinRoom={handleJoinRoom}
           isConnecting={isConnecting}
           connectionError={connectionError}
         />
      ) : (
           <RoomPage
           peerId={peerId}
           peerRole={peerRole}
           peers={peers}
           isConnected={isConnected}
           canControlTorrent={peerRole === "master"}
           magnetLink={magnetLink}
           torrentFileName={torrentFile?.name ?? null}
           sharedSourceLabel={sharedTorrentLabel}
           mediaFiles={mediaFiles}
           selectedMediaIndex={selectedMediaIndex}
           selectedMediaLabel={selectedMediaLabel}
           selectedMediaKind={selectedMediaKind}
           selectedMediaAudioTracks={selectedMediaAudioTracks}
           selectedAudioTrackIndex={selectedAudioTrackIndex}
           selectedSubtitles={selectedSubtitles}
           selectedSubtitleIndex={selectedSubtitleIndex}
          onSubtitleTrackChange={handleSubtitleTrackChange}
          torrentPeerCount={torrentPeerCount}
          syncToleranceSeconds={syncToleranceSeconds}
          onSyncToleranceChange={handleSyncToleranceChange}
          onMagnetLinkChange={setMagnetLink}
          onTorrentFileChange={handleTorrentFileChange}
          videoRef={videoRef}
          playbackNotice={playbackNotice}
          onPlaybackStarted={() => setPlaybackNotice(null)}
          onAudioTrackChange={handleAudioTrackChange}
          onPlayerReady={(ready) => {
            isPlayerReadyRef.current = ready;
            setIsPlayerReady(ready);
            if (ready) tryApplyPendingRemoteSync();
          }}
          onLoadMagnet={() => void handleLoadMagnet()}
          onLoadTorrentFile={() => void handleLoadTorrentFile()}
          onSelectMediaFile={handleSelectMediaFile}
          onLeaveRoom={handleLeaveRoom}
          onResetTorrentInRoom={handleResetTorrentInRoom}
          isLoadingTorrent={isLoadingTorrent}
          downloadSpeed={downloadSpeed}
          bufferingProgress={selectedMediaBufferProgress}
          torrentError={torrentError}
          torrentPeerHint={torrentPeerHint}
          bufferHint={bufferHint}
          trackerLost={trackerLost}
          onTimeUpdate={handleTimeUpdate}
          onBufferingChange={handleBufferingChange}
          bufferWindowMB={bufferWindowMB}
          maxBufferMB={maxBufferMB}
          onBufferSettingsChange={handleBufferSettingsChange}
          onSeek={handleSeek}
          onMuxStreamRequest={handleMuxStreamRequest}
          connectionQuality={connectionQuality}
          rttMs={rttMs}
          onShowLeaveConfirm={() => setShowLeaveConfirm(true)}
          onShowResetConfirm={() => setShowResetConfirm(true)}
          onReturnHome={() => setCurrentView("home")}
          chatMessages={chatMessages}
          onSendChat={handleSendChat}
        />
      )}
      <ConfirmModal
        isOpen={showLeaveConfirm}
        title="Leave room?"
        message="Are you sure you want to leave the room? All connections will be closed."
        confirmLabel="Leave"
        danger
        onConfirm={handleLeaveRoom}
        onCancel={() => setShowLeaveConfirm(false)}
      />
      <ConfirmModal
        isOpen={showResetConfirm}
        title="Change torrent source?"
        message="Current playback will stop and the room will be reset."
        confirmLabel="Change"
        danger
        onConfirm={handleResetTorrentInRoom}
        onCancel={() => setShowResetConfirm(false)}
      />
      <ConfirmModal
        isOpen={showCloseConfirm}
        title="Close application?"
        message="Are you sure you want to close TorrSyncPlayer? All connections will be lost."
        confirmLabel="Close"
        danger
        onConfirm={handleCloseConfirmed}
        onCancel={handleCloseCancelled}
      />
    </main>
  );
}

export default App;
