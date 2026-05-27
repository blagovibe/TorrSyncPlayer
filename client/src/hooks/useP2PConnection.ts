import { useCallback, useEffect, useRef, useState } from "react";
import P2PService from "../services/P2PService";
import {
  type ChatMessage,
  type ConnectionQuality,
  type Peer,
  type PeerConnectionState,
  type PeerRole,
  type SyncMessage,
  type TorrentSourceMessage,
} from "../services/types";
import { type RoomConfigMessage } from "../services/types";
import { uiLogger } from "../utils/logger";
import { UI_CONFIG } from "../config";
import { useRoomStateContext } from "./useRoomStateContext";

export interface P2PConnection {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  isConnecting: boolean;
  connectionError: string | null;
  connectionQuality: ConnectionQuality;
  rttMs: number | null;
  reconnectFailed: boolean;
  chatMessages: ChatMessage[];
  p2pService: P2PService | null;
  createRoom: () => Promise<void>;
  joinRoom: (code: string) => Promise<void>;
  disconnect: () => Promise<void>;
  sendChat: (text: string) => void;
  broadcastRoomState: (targetPeerId?: string) => void;
  scheduleBroadcast: (targetPeerId?: string) => void;
  onSync: (cb: (msg: SyncMessage) => void) => () => void;
  onTorrentSource: (cb: (msg: TorrentSourceMessage) => void) => () => void;
  onRoomConfig: (cb: (msg: RoomConfigMessage) => void) => () => void;
}

export function useP2PConnection(): P2PConnection {
  const roomState = useRoomStateContext();
  const { setPendingSync, setPeerRole: setCtxPeerRole, setSyncToleranceSeconds: setCtxSyncTolerance } = roomState;

  const [peerId, setPeerId] = useState("");
  const [peerRole, setPeerRole] = useState<PeerRole | null>(null);
  const [peers, setPeers] = useState<Peer[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [connectionQuality, setConnectionQuality] = useState<ConnectionQuality>("unknown");
  const [rttMs, setRttMs] = useState<number | null>(null);
  const [reconnectFailed, setReconnectFailed] = useState(false);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);

  const p2pServiceRef = useRef<P2PService | null>(null);
  const syncListeners = useRef(new Set<(msg: SyncMessage) => void>());
  const torrentSourceListeners = useRef(new Set<(msg: TorrentSourceMessage) => void>());
  const roomConfigListeners = useRef(new Set<(msg: RoomConfigMessage) => void>());
  const broadcastFn = useRef<(targetPeerId?: string) => void>(() => {});

  const setupListeners = useCallback((svc: P2PService, role: "host" | "guest") => {
    svc.on("connected", () => {
      setIsConnected(true);
      setIsConnecting(false);
      setPeers((prev) => prev.map((p) =>
        p.id !== "self" ? { ...p, connectionState: "connected" as PeerConnectionState } : p,
      ));
    });
    svc.on("disconnected", () => {
      setIsConnected(false);
      setPeers((prev) => prev.map((p) => ({ ...p, connectionState: "disconnected" as PeerConnectionState })));
      setPendingSync(null);
    });
    svc.on("peer_connected", (id) => {
      setPeers((prev) => prev.some((p) => p.id === id) ? prev : [...prev, { id, name: "Peer", role: "slave", connectionState: "connected" }]);
      if (role === "host") broadcastFn.current(id);
    });
    svc.on("peer_disconnected", (id) => {
      setPeers((prev) => prev.filter((p) => p.id !== id));
      svc.clearRateLimitForPeer(id);
    });
    svc.on("sync", (msg) => {
      if (roomState.state.peerRole !== "slave") return;
      setPendingSync(msg);
      syncListeners.current.forEach((cb) => cb(msg));
    });
    svc.on("torrent_source", (msg) => {
      if (roomState.state.peerRole !== "slave") return;
      torrentSourceListeners.current.forEach((cb) => cb(msg));
    });
    svc.on("room_config", (msg) => {
      const t = Math.min(Math.max(msg.syncToleranceSeconds, 0), 30);
      setCtxSyncTolerance(t);
      roomConfigListeners.current.forEach((cb) => cb(msg));
    });
    svc.on("reconnecting", (attempt, delay) => {
      setConnectionError(`Connection lost. Reconnecting in ${Math.round(delay / 1000)}s... (attempt ${attempt})`);
    });
    svc.on("error", (err) => { setConnectionError(err.message); setIsConnecting(false); });
    svc.on("connection_quality", (q) => { setConnectionQuality(q); setRttMs(svc.getLastRttMs()); });
    svc.on("reconnect_failed", () => { setReconnectFailed(true); });
    svc.on("resend_requested", (id) => { broadcastFn.current(id); });
    svc.on("chat_received", (senderId, content) => {
      if (typeof content !== "string" || typeof senderId !== "string") return;
      const trimmed = content.trim();
      if (!trimmed || trimmed.length > 500) return;
      setChatMessages((prev) => [...prev, { id: crypto.randomUUID(), sender: senderId, text: trimmed, timestamp: Date.now() }].slice(-UI_CONFIG.maxChatMessages));
    });

    broadcastFn.current = (targetPeerId?: string) => {
      if (!svc.isHost()) return;
      const torrentSource = roomState.state.currentTorrentSource;
      if (torrentSource) {
        svc.sendTorrentSource(torrentSource, roomState.state.selectedMediaIndex, roomState.state.selectedAudioTrackIndex, roomState.state.selectedSubtitleIndex, targetPeerId);
      }
      svc.sendRoomConfig(roomState.state.syncToleranceSeconds, targetPeerId);
    };
  }, [setPendingSync, setCtxSyncTolerance, roomState]);

  const initialize = useCallback(async (role: "host" | "guest") => {
    p2pServiceRef.current?.disconnect();
    setPendingSync(null);
    const svc = new P2PService();
    if (role === "host") {
      svc.setHost();
    } else {
      svc.setGuest();
    }
    setupListeners(svc, role);
    p2pServiceRef.current = svc;
    await svc.initialize();
    setPeerId(svc.getPeerId());
    return svc;
  }, [setupListeners, setPendingSync]);

  const createRoom = useCallback(async () => {
    if (isConnecting) return;
    setIsConnecting(true);
    setConnectionError(null);
    setReconnectFailed(false);
    try {
      await initialize("host");
      setPeerRole("master");
      setCtxPeerRole("master");
      setPeers([{ id: "self", name: "You", role: "master", connectionState: "connected" }]);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Unable to initialize room";
      uiLogger.error("Failed to create room:", err);
      try { p2pServiceRef.current?.disconnect(); } catch (e) { uiLogger.warn("Cleanup failed:", e); }
      p2pServiceRef.current = null;
      setConnectionError(msg);
      setPeerRole(null);
      setCtxPeerRole(null);
      setPeers([]);
      setIsConnected(false);
    } finally { setIsConnecting(false); }
  }, [isConnecting, initialize, setCtxPeerRole]);

  const joinRoom = useCallback(async (code: string) => {
    if (isConnecting) return;
    const normalized = code.trim().toUpperCase();
    if (!normalized || normalized.length !== 6 || !/^[A-Z0-9]+$/.test(normalized)) {
      setConnectionError("Invalid peer ID");
      return;
    }
    setIsConnecting(true);
    setConnectionError(null);
    setReconnectFailed(false);
    try {
      const svc = await initialize("guest");
      const timeout = new Promise<never>((_, rej) => setTimeout(() => rej(new Error("Connection timed out.")), 60_000));
      await Promise.race([svc.connect(`torrsync-${normalized}`), timeout]);
      setPeerRole("slave");
      setCtxPeerRole("slave");
      setIsConnected(true);
      setPeers([
        { id: "self", name: "You", role: "slave", connectionState: "connected" },
        { id: normalized, name: "Host", role: "master", connectionState: "connected" },
      ]);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Connection failed";
      uiLogger.error("Failed to connect:", err);
      try { p2pServiceRef.current?.disconnect(); } catch (e) { uiLogger.warn("Cleanup:", e); }
      p2pServiceRef.current = null;
      setConnectionError(msg);
      setPeerRole(null);
      setCtxPeerRole(null);
      setPeers([]);
      setIsConnected(false);
    } finally { setIsConnecting(false); }
  }, [isConnecting, initialize, setCtxPeerRole]);

  const disconnect = useCallback(async () => {
    try { await p2pServiceRef.current?.disconnect(); } catch (e) { uiLogger.warn("Disconnect:", e); }
    p2pServiceRef.current = null;
    setPeerId(""); setPeerRole(null); setCtxPeerRole(null); setPeers([]);
    setIsConnected(false); setIsConnecting(false);
    setConnectionError(null); setPendingSync(null);
    setReconnectFailed(false); setChatMessages([]);
  }, [setPendingSync, setCtxPeerRole]);

  const sendChat = useCallback((text: string) => {
    if (!text.trim() || !p2pServiceRef.current) return;
    const trimmed = text.trim();
    if (trimmed.length > 500) return;
    const msg = { id: crypto.randomUUID(), sender: peerId, text: trimmed, timestamp: Date.now() };
    setChatMessages((prev) => [...prev, msg].slice(-UI_CONFIG.maxChatMessages));
    p2pServiceRef.current.sendChat(msg.text);
  }, [peerId]);

  const scheduleBroadcast = useCallback((targetPeerId?: string) => {
    broadcastFn.current(targetPeerId);
  }, []);

  const broadcastRoomState = useCallback((targetPeerId?: string) => {
    broadcastFn.current(targetPeerId);
  }, []);

  useEffect(() => {
    const cleanup = () => { p2pServiceRef.current?.disconnect(); p2pServiceRef.current = null; };
    window.addEventListener("beforeunload", cleanup);
    return () => { window.removeEventListener("beforeunload", cleanup); cleanup(); };
  }, []);

  return {
    peerId, peerRole, peers, isConnected, isConnecting, connectionError,
    connectionQuality, rttMs, reconnectFailed, chatMessages,
    p2pService: p2pServiceRef.current, createRoom, joinRoom, disconnect,
    sendChat, broadcastRoomState, scheduleBroadcast,
    onSync: (cb: (msg: SyncMessage) => void) => { syncListeners.current.add(cb); return () => { syncListeners.current.delete(cb); }; },
    onTorrentSource: (cb: (msg: TorrentSourceMessage) => void) => { torrentSourceListeners.current.add(cb); return () => { torrentSourceListeners.current.delete(cb); }; },
    onRoomConfig: (cb: (msg: RoomConfigMessage) => void) => { roomConfigListeners.current.add(cb); return () => { roomConfigListeners.current.delete(cb); }; },
  };
}
