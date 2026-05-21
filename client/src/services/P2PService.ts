import { Peer, type DataConnection, type PeerJSOption } from "peerjs";
import { createCleanup, type CleanupHandle } from "../utils/cleanup";
import { p2pLogger } from "../utils/logger";
import { P2P_CONFIG } from "../config";
import {
  type ConnectionQuality,
  type RoomConfigMessage,
  type SharedTorrentSource,
  type SyncMessage,
} from "./types";

type TorrentSourceMessage = {
  source: SharedTorrentSource;
  selectedMediaIndex: number | null;
  selectedAudioTrackIndex: number | null;
  selectedSubtitleIndex: number | null;
};

type P2PEvents = {
  connected: () => void;
  disconnected: () => void;
  peer_connected: (peerId: string) => void;
  peer_disconnected: (peerId: string) => void;
  sync: (message: SyncMessage) => void;
  torrent_source: (message: TorrentSourceMessage) => void;
  room_config: (message: RoomConfigMessage) => void;
  error: (error: Error) => void;
  reconnecting: (attempt: number, delayMs: number) => void;
  connection_quality: (quality: ConnectionQuality) => void;
};

type EventKey = keyof P2PEvents;

type OutboundMessage =
  | { type: "sync"; message: SyncMessage }
  | {
      type: "torrent_source";
      source: SharedTorrentSource;
      selectedMediaIndex: number | null;
      selectedAudioTrackIndex: number | null;
      selectedSubtitleIndex: number | null;
    }
  | { type: "room_config"; syncToleranceSeconds: number; roomPassword?: string }
  | { type: "ping"; ts: number }
  | { type: "pong"; ts: number };

export const WEBRTC_UNAVAILABLE_MESSAGE =
  "WebRTC data channels are not available in the current desktop runtime. Use the Electron build, which ships Chromium with WebRTC support.";
export const SIGNALING_UNAVAILABLE_MESSAGE =
  "Unable to reach the PeerJS signaling server. Check your internet connection or configure a reachable PeerJS server with VITE_PEERJS_HOST.";

type PeerServerEnv = {
  VITE_PEERJS_HOST?: string;
  VITE_PEERJS_PORT?: string;
  VITE_PEERJS_PATH?: string;
  VITE_PEERJS_SECURE?: string;
};

function generatePeerId(): string {
  const bytes = new Uint8Array(P2P_CONFIG.peerIdLength);
  crypto.getRandomValues(bytes);
  let id = "";
  for (let i = 0; i < P2P_CONFIG.peerIdLength; i++) {
    id += P2P_CONFIG.peerIdChars.charAt(bytes[i] % P2P_CONFIG.peerIdChars.length);
  }
  return id;
}

function getErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

export function getWebRTCSupportIssue(): string | null {
  if (typeof RTCPeerConnection === "undefined") {
    return "RTCPeerConnection is not exposed by the current WebView";
  }

  let testConnection: RTCPeerConnection | null = null;
  let testChannel: RTCDataChannel | null = null;
  try {
    testConnection = new RTCPeerConnection({ iceServers: [] });
    if (typeof testConnection.createDataChannel !== "function") {
      return "RTCDataChannel is not exposed by the current WebView";
    }
    testChannel = testConnection.createDataChannel("_torrsync_webrtc_test");
    return null;
  } catch (error) {
    return `RTCPeerConnection failed to initialize: ${getErrorMessage(error)}`;
  } finally {
    testChannel?.close();
    testConnection?.close();
  }
}

function createWebRTCUnavailableError(issue: string): Error {
  return new Error(`${WEBRTC_UNAVAILABLE_MESSAGE} (${issue})`);
}

function createSignalingUnavailableError(message: string): Error {
  return new Error(`${SIGNALING_UNAVAILABLE_MESSAGE} (${message})`);
}

function getImportMetaEnv(): PeerServerEnv {
  return ((import.meta as ImportMeta & { env?: PeerServerEnv }).env ?? {}) as PeerServerEnv;
}

function normalizePeerPath(path: string): string {
  let normalizedPath = path.trim() || P2P_CONFIG.defaultPath;
  if (!normalizedPath.startsWith("/")) normalizedPath = `/${normalizedPath}`;
  if (!normalizedPath.endsWith("/")) normalizedPath = `${normalizedPath}/`;
  return normalizedPath;
}

function parsePeerPort(port: string | undefined): number {
  if (!port?.trim()) return P2P_CONFIG.defaultPort;
  const parsedPort = Number(port);
  if (!Number.isInteger(parsedPort) || parsedPort <= 0 || parsedPort > 65535) {
    return P2P_CONFIG.defaultPort;
  }
  return parsedPort;
}

function parsePeerSecure(secure: string | undefined, host: string): boolean {
  if (secure === "false" || secure === "0") return false;
  if (secure === "true" || secure === "1") return true;
  return host === P2P_CONFIG.defaultHost || location.protocol === "https:";
}

function isSyncMessage(message: Partial<SyncMessage>): message is SyncMessage {
  return (
    typeof message.action === "string" &&
    typeof message.position === "number" &&
    typeof message.server_ts === "number"
  );
}

function parseInboundMessage(rawData: unknown): OutboundMessage | null {
  let candidate: unknown = rawData;
  if (typeof rawData === "string") {
    try {
      candidate = JSON.parse(rawData);
    } catch {
      return null;
    }
  }
  if (!candidate || typeof candidate !== "object") return null;

  const message = candidate as Partial<OutboundMessage> & { type?: string };

  switch (message.type) {
    case "sync":
      if (message.message && isSyncMessage(message.message)) {
        return { type: "sync", message: message.message };
      }
      return null;
    case "torrent_source": {
      if (!message.source) return null;
      const src = message.source as { magnetLink?: unknown; fileName?: unknown; bytes?: unknown; sourceKey?: unknown };
      if (src.magnetLink !== undefined && typeof src.magnetLink !== "string") return null;
      if (src.fileName !== undefined && typeof src.fileName !== "string") return null;
      if (src.sourceKey !== undefined && typeof src.sourceKey !== "string") return null;
      const smi = message.selectedMediaIndex;
      const sati = message.selectedAudioTrackIndex;
      const ssti = message.selectedSubtitleIndex;
      if ((typeof smi !== "number" && smi !== null) || (typeof sati !== "number" && sati !== null) || (typeof ssti !== "number" && ssti !== null)) {
        return null;
      }
      // Validate source.bytes if present (must be array of numbers 0-255)
      if (src.bytes !== undefined) {
        if (!Array.isArray(src.bytes) || !src.bytes.every((b: unknown) => typeof b === "number" && b >= 0 && b <= 255)) {
          return null;
        }
      }
      return {
        type: "torrent_source",
        source: message.source as SharedTorrentSource,
        selectedMediaIndex: smi,
        selectedAudioTrackIndex: sati,
        selectedSubtitleIndex: ssti,
      };
    }
    case "room_config":
      if (typeof message.syncToleranceSeconds === "number" && Number.isFinite(message.syncToleranceSeconds)) {
        return {
          type: "room_config",
          syncToleranceSeconds: message.syncToleranceSeconds,
          roomPassword: typeof message.roomPassword === "string" ? message.roomPassword : undefined,
        };
      }
      return null;
    case "ping":
      if (typeof message.ts === "number") {
        return { type: "ping", ts: message.ts };
      }
      return null;
    case "pong":
      if (typeof message.ts === "number") {
        return { type: "pong", ts: message.ts };
      }
      return null;
    default:
      return null;
  }
}

export function buildPeerServerOptions(env: PeerServerEnv): PeerJSOption {
  const host = env.VITE_PEERJS_HOST?.trim() || P2P_CONFIG.defaultHost;
  return {
    host,
    port: parsePeerPort(env.VITE_PEERJS_PORT),
    path: normalizePeerPath(env.VITE_PEERJS_PATH ?? P2P_CONFIG.defaultPath),
    secure: parsePeerSecure(env.VITE_PEERJS_SECURE, host),
    debug: 1,
  };
}

export function getPeerServerOptions(): PeerJSOption {
  return buildPeerServerOptions(getImportMetaEnv());
}

function normalizePeerError(error: unknown): Error {
  const peerError = error as { type?: string; message?: string };
  const message = peerError.message ?? getErrorMessage(error);

  if (peerError.type === "browser-incompatible" || message.includes("The current browser does not support WebRTC")) {
    return createWebRTCUnavailableError("PeerJS could not initialize WebRTC data channels");
  }
  if (peerError.type === "network" || message.includes("Lost connection to server")) {
    return createSignalingUnavailableError(message);
  }
  return error instanceof Error ? error : new Error(message);
}

type P2PServiceState = 
  | "disconnected"
  | "connecting"
  | "connected"
  | "disconnecting"
  | "destroyed";

export class P2PService {
  private peerId: string;
  private role: "host" | "guest" = "host";
  private peer: Peer | null = null;
  private connections = new Map<string, DataConnection>();
  private remotePeerId: string | null = null;
  private state: P2PServiceState = "disconnected";
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private lastRttMs: number | null = null;
  private pingIntervalId: ReturnType<typeof setInterval> | null = null;
  private readonly cleanup: CleanupHandle;
  private listeners: { [K in EventKey]: Set<P2PEvents[K]> } = {
    connected: new Set(),
    disconnected: new Set(),
    peer_connected: new Set(),
    peer_disconnected: new Set(),
    sync: new Set(),
    torrent_source: new Set(),
    room_config: new Set(),
    error: new Set(),
    reconnecting: new Set(),
    connection_quality: new Set(),
  };

  constructor() {
    this.peerId = generatePeerId();
    this.cleanup = createCleanup();
  }

  getState(): P2PServiceState {
    return this.state;
  }

  isConnected(): boolean {
    return this.state === "connected";
  }

  isInRoom(): boolean {
    return this.peer !== null && this.state !== "disconnected" && this.state !== "destroyed";
  }

  on<K extends EventKey>(event: K, callback: P2PEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  async connect(remotePeerId: string): Promise<void> {
    if (this.state === "connecting") {
      throw new Error("Connection already in progress");
    }
    if (this.state === "destroyed") {
      throw new Error("P2PService has been destroyed");
    }
    const existing = this.getConnection(remotePeerId);
    if (existing?.open) return;

    const maxRetries = P2P_CONFIG.connectRetryAttempts;
    const baseDelay = P2P_CONFIG.connectRetryBaseDelayMs;
    let lastError: Error | null = null;

    for (let attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        await this.tryConnect(remotePeerId);
        return;
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));
        p2pLogger.warn(`Connection attempt ${attempt}/${maxRetries} failed: ${lastError.message}`);
        if (attempt < maxRetries) {
          const delay = baseDelay * Math.pow(2, attempt - 1);
          await new Promise<void>((resolve) => { setTimeout(resolve, delay); });
        }
      }
    }

    throw lastError ?? new Error("Connection failed after retries");
  }

  private async tryConnect(remotePeerId: string): Promise<void> {
    this.remotePeerId = remotePeerId;
    this.state = "connecting";

    try {
      return await new Promise((resolve, reject) => {
        if (!this.peer) {
          reject(new Error("Peer not initialized"));
          return;
        }

        const conn = this.peer.connect(remotePeerId, {
          reliable: true,
          serialization: "json",
        });

        let isSettled = false;
        let timeoutId: ReturnType<typeof setTimeout> | null = null;

        const settleResolve = () => {
          if (isSettled) return;
          isSettled = true;
          this.state = "connected";
          if (timeoutId !== null) {
            clearTimeout(timeoutId);
            timeoutId = null;
          }
          if (!conn.open) {
            reject(new Error("Connection closed before it was established"));
            return;
          }
          if (this.getOpenConnectionCount() === 1) {
            this.emit("connected");
          }
          resolve();
        };
        const settleReject = (error: Error) => {
          if (isSettled) return;
          isSettled = true;
          this.state = "disconnected";
          if (timeoutId !== null) {
            clearTimeout(timeoutId);
            timeoutId = null;
          }
          reject(error);
        };

        timeoutId = this.cleanup.setTimeout(() => {
          timeoutId = null;
          if (!isSettled) {
            conn.close();
            settleReject(new Error("Connection timeout"));
          }
        }, P2P_CONFIG.connectionTimeoutMs);

        this.bindConnection(remotePeerId, conn, {
          emitPeerConnected: true,
          onOpen: () => {
            settleResolve();
          },
          onClose: () => {
            settleReject(new Error("Connection closed before it was established"));
          },
        });
      });
    } catch (error) {
      this.state = "disconnected";
      throw error;
    }
  }

  initialize(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.peer) {
        resolve();
        return;
      }
      if (this.isDestroyed) {
        reject(new Error("P2PService has been destroyed"));
        return;
      }

      const webRTCSupportIssue = getWebRTCSupportIssue();
      if (webRTCSupportIssue) {
        reject(createWebRTCUnavailableError(webRTCSupportIssue));
        return;
      }

      let isSettled = false;
      const settleResolve = () => {
        if (isSettled) return;
        isSettled = true;
        resolve();
      };
      const settleReject = (error: Error) => {
        if (isSettled) return;
        isSettled = true;
        reject(error);
      };

      const peerServerOptions = getPeerServerOptions();
      if (this.role === "host") {
        this.peer = new Peer(`${P2P_CONFIG.hostPeerPrefix}${this.peerId}`, peerServerOptions);
      } else {
        this.peer = new Peer(peerServerOptions);
      }

      const onOpen = (id: string) => {
        p2pLogger.info("PeerJS initialized with ID:", id);
        clearTimeout(initTimeoutId);
        if (id) {
          this.peerId = id.startsWith(P2P_CONFIG.hostPeerPrefix)
            ? id.slice(P2P_CONFIG.hostPeerPrefix.length)
            : id;
        }
        settleResolve();
      };
      this.peer.on("open", onOpen);
      this.cleanup.add(() => { this.peer?.off?.("open", onOpen); });

      const onConnection = (conn: DataConnection) => {
        this.handleIncomingConnection(conn);
      };
      this.peer.on("connection", onConnection);
      this.cleanup.add(() => { this.peer?.off?.("connection", onConnection); });

      const onError = (err: Error) => {
        const error = normalizePeerError(err);
        p2pLogger.error("PeerJS error:", err);
        this.emit("error", error);
        if (!isSettled) {
          const peerErr = err as { type?: string };
          if (peerErr.type === "peer-unavailable") {
            settleReject(new Error("Peer not found. Check the room code and try again."));
          } else if (peerErr.type === "network" || peerErr.type === "server-error" || peerErr.type === "socket-error") {
            settleReject(new Error("Unable to reach the signaling server. Check your internet connection."));
          } else if (peerErr.type === "browser-incompatible") {
            settleReject(createWebRTCUnavailableError("PeerJS could not initialize WebRTC data channels"));
          } else {
            settleReject(error);
          }
        }
      };
      this.peer.on("error", onError);
      this.cleanup.add(() => { this.peer?.off?.("error", onError); });

      const onDisconnected = () => {
        if (this.isDisconnecting) return;
        this.emit("disconnected");
        this.attemptReconnect();
      };
      this.peer.on("disconnected", onDisconnected);
      this.cleanup.add(() => { this.peer?.off?.("disconnected", onDisconnected); });

      const initTimeoutId = this.cleanup.setTimeout(() => {
        if (!this.peer?.open && !isSettled) {
          settleReject(new Error("Initialization timeout"));
        }
      }, P2P_CONFIG.initTimeoutMs);
    });
  }

  private attemptReconnect(): void {
    if (this.state === "destroyed" || this.state === "disconnecting" || this.state === "reconnecting") return;
    if (this.role === "host") return;
    if (!this.remotePeerId) return;
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      p2pLogger.warn("Max reconnect attempts reached, giving up");
      this.emit("error", new Error("Connection lost. Please rejoin the room."));
      return;
    }

    this.state = "reconnecting";
    this.reconnectAttempts++;
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), 30_000);
    p2pLogger.info(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
    this.emit("reconnecting", this.reconnectAttempts, delay);

    this.cleanup.setTimeout(async () => {
      if (this.state === "destroyed" || this.state === "disconnecting") {
        this.state = "disconnected";
        return;
      }
      try {
        if (this.peer && !this.peer.destroyed) {
          this.peer.reconnect();
        }
        if (this.remotePeerId) {
          await this.connect(this.remotePeerId);
        }
        this.reconnectAttempts = 0;
        this.state = "disconnected"; // Will be updated to "connected" if successful
      } catch (error) {
        this.state = "disconnected";
        p2pLogger.warn("Reconnect failed:", error);
        this.attemptReconnect();
      }
    }, delay);
  }

  sendSync(message: SyncMessage, targetPeerId?: string): void {
    this.sendPayload({ type: "sync", message }, targetPeerId);
  }

  sendTorrentSource(
    payload: {
      source: SharedTorrentSource;
      selectedMediaIndex: number | null;
      selectedAudioTrackIndex: number | null;
      selectedSubtitleIndex: number | null;
    },
    targetPeerId?: string,
  ): void {
    this.sendPayload({ type: "torrent_source", ...payload }, targetPeerId);
  }

  sendRoomConfig(payload: RoomConfigMessage, targetPeerId?: string): void {
    this.sendPayload({
      type: "room_config",
      syncToleranceSeconds: payload.syncToleranceSeconds,
      roomPassword: payload.roomPassword,
    }, targetPeerId);
  }

  disconnect(): void {
    if (this.state === "disconnecting" || this.state === "disconnected" || this.state === "destroyed") return;
    
    this.state = "disconnecting";
    this.stopPingInterval();
    this.cleanup.abort();

    for (const connection of this.connections.values()) {
      connection.close();
    }
    this.connections.clear();

    if (this.peer) {
      this.peer.destroy();
      this.peer = null;
    }

    this.remotePeerId = null;
    this.emit("disconnected");
    this.state = "disconnected";
  }

  /**
   * Full destruction. After calling this, the service cannot be reused.
   */
  destroy(): void {
    this.state = "destroyed";
    this.disconnect();
    for (const key of Object.keys(this.listeners) as EventKey[]) {
      this.listeners[key].clear();
    }
  }

  isConnected(): boolean {
    return this.getOpenConnectionCount() > 0;
  }

  isInRoom(): boolean {
    return this.peer !== null && this.state !== "disconnected" && this.state !== "destroyed";
  }

  getRemotePeerId(): string | null {
    return this.remotePeerId ?? (this.connections.size > 0 ? this.connections.keys().next().value ?? null : null);
  }

  private handleIncomingConnection(conn: DataConnection): void {
    this.remotePeerId = conn.peer;
    this.bindConnection(conn.peer, conn, {
      emitPeerConnected: true,
      onOpen: () => {
        if (this.getOpenConnectionCount() === 1) {
          this.emit("connected");
        }
      },
    });
  }

  private bindConnection(
    peerId: string,
    conn: DataConnection,
    options: {
      emitPeerConnected: boolean;
      onOpen?: () => void;
      onClose?: () => void;
    },
  ): void {
    const existingConnection = this.connections.get(peerId);
    if (existingConnection && existingConnection !== conn) {
      existingConnection.close();
    }

    this.connections.set(peerId, conn);

    const wasAlreadyOpen = conn.open;

    const onConnOpen = () => {
      if (this.connections.get(peerId) !== conn) return;
      if (this.isDisconnecting) return;
      if (options.emitPeerConnected) this.emit("peer_connected", peerId);
      options.onOpen?.();
      this.startPingInterval();
    };

    if (wasAlreadyOpen) {
      if (this.isDisconnecting) return;
      if (options.emitPeerConnected) this.emit("peer_connected", peerId);
      options.onOpen?.();
      this.startPingInterval();
    } else {
      conn.on("open", onConnOpen);
      this.cleanup.add(() => { conn.off?.("open", onConnOpen); });
    }

    const onConnData = (data: unknown) => {
      if (this.connections.get(peerId) !== conn) return;
      const message = parseInboundMessage(data);
      if (!message) return;
      switch (message.type) {
        case "sync":
          this.emit("sync", message.message);
          break;
        case "torrent_source":
          this.emit("torrent_source", {
            source: message.source,
            selectedMediaIndex: message.selectedMediaIndex,
            selectedAudioTrackIndex: message.selectedAudioTrackIndex,
            selectedSubtitleIndex: message.selectedSubtitleIndex,
          });
          break;
        case "room_config":
          this.emit("room_config", {
            syncToleranceSeconds: message.syncToleranceSeconds,
            roomPassword: message.roomPassword,
          });
          break;
        case "ping":
          // Respond with pong, echoing the timestamp
          this.sendPayload({ type: "pong", ts: message.ts });
          break;
        case "pong":
          this.handlePong(message.ts);
          break;
      }
    };
    conn.on("data", onConnData);
    this.cleanup.add(() => { conn.off?.("data", onConnData); });

    const onConnClose = () => {
      const trackedConnection = this.connections.get(peerId);
      if (trackedConnection === conn) {
        this.connections.delete(peerId);
      }
      if (this.remotePeerId === peerId) {
        this.remotePeerId = null;
      }
      if (this.isDisconnecting) return;
      this.emit("peer_disconnected", peerId);
      options.onClose?.();
      if (this.getOpenConnectionCount() === 0) {
        this.emit("disconnected");
      }
    };
    conn.on("close", onConnClose);
    this.cleanup.add(() => { conn.off?.("close", onConnClose); });

    const onConnError = (err: Error) => {
      if (this.connections.get(peerId) !== conn) return;
      this.emit("error", err instanceof Error ? err : new Error(String(err)));
    };
    conn.on("error", onConnError);
    this.cleanup.add(() => { conn.off?.("error", onConnError); });
  }

  private sendPayload(payload: OutboundMessage, targetPeerId?: string): void {
    if (!this.isConnected()) {
      p2pLogger.warn("sendPayload dropped: not connected", payload.type);
      return;
    }
    const targetConnections = targetPeerId
      ? (() => {
          const connection = this.connections.get(targetPeerId);
          return connection && connection.open ? [connection] : [];
        })()
      : Array.from(this.connections.values()).filter((connection) => connection.open === true);

    for (const connection of targetConnections) {
      connection.send(payload);
    }
  }

  private getConnection(peerId: string): DataConnection | null {
    return this.connections.get(peerId) ?? null;
  }

  private getOpenConnectionCount(): number {
    let count = 0;
    for (const connection of this.connections.values()) {
      if (connection.open) count += 1;
    }
    return count;
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<P2PEvents[K]>): void {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<P2PEvents[K]>) => void)(...args);
    }
  }

  private startPingInterval(): void {
    this.stopPingInterval();
    this.pingIntervalId = this.cleanup.setInterval(() => {
      this.sendPing();
    }, 5000);
  }

  private stopPingInterval(): void {
    if (this.pingIntervalId !== null) {
      clearInterval(this.pingIntervalId);
      this.pingIntervalId = null;
    }
  }

  private sendPing(): void {
    if (!this.isConnected()) return;
    const pingPayload = { type: "ping", ts: Date.now() };
    const targetConnections = Array.from(this.connections.values()).filter((c) => c.open);
    for (const connection of targetConnections) {
      try {
        connection.send(pingPayload);
      } catch {
        // Connection may have closed
      }
    }
  }

  private handlePong(pongTs: number): void {
    const rtt = Date.now() - pongTs;
    if (rtt >= 0) {
      this.lastRttMs = rtt;
      this.emit("connection_quality", this.getConnectionQuality());
    }
  }
}

export default P2PService;
