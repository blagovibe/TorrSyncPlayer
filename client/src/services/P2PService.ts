import { Peer, type DataConnection, type PeerJSOption } from "peerjs";
import { createCleanup, type CleanupHandle } from "../utils/cleanup";
import { p2pLogger } from "../utils/logger";
import { P2P_CONFIG, P2P_MAX_TORRENT_BYTES } from "../config";
import {
  type ConnectionQuality,
  type RoomConfigMessage,
  type SharedTorrentSource,
  type SyncMessage,
  type TorrentSourceMessage,
} from "./types";

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
  chat_received: (senderId: string, content: string) => void;
  resend_requested: (peerId: string) => void;
  reconnect_failed: () => void;
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
  | { type: "room_config"; syncToleranceSeconds: number }
  | { type: "ping"; ts: number }
  | { type: "pong"; ts: number }
  | { type: "chat"; content: string };

export const WEBRTC_UNAVAILABLE_MESSAGE =
  "WebRTC data channels are not available in the current desktop runtime. Use the Electron build, which ships Chromium with WebRTC support.";
export const SIGNALING_UNAVAILABLE_MESSAGE =
  "Unable to reach the PeerJS signaling server. Check your internet connection or configure a reachable PeerJS server with VITE_PEERJS_HOST.";

interface PeerServerEnv {
  VITE_PEERJS_HOST?: string;
  VITE_PEERJS_PORT?: string;
  VITE_PEERJS_PATH?: string;
  VITE_PEERJS_SECURE?: string;
}

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
  const env = import.meta.env;
  return {
    VITE_PEERJS_HOST: env.VITE_PEERJS_HOST,
    VITE_PEERJS_PORT: env.VITE_PEERJS_PORT,
    VITE_PEERJS_PATH: env.VITE_PEERJS_PATH,
    VITE_PEERJS_SECURE: env.VITE_PEERJS_SECURE,
  };
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
      if (src.magnetLink !== undefined && (typeof src.magnetLink !== "string" || src.magnetLink.length > 8000)) return null;
      if (src.fileName !== undefined && (typeof src.fileName !== "string" || src.fileName.length > 1024)) return null;
      if (src.sourceKey !== undefined && (typeof src.sourceKey !== "string" || src.sourceKey.length > 2048)) return null;
      const smi = message.selectedMediaIndex;
      const sati = message.selectedAudioTrackIndex;
      const ssti = message.selectedSubtitleIndex;
      if ((typeof smi !== "number" && smi !== null) || (typeof sati !== "number" && sati !== null) || (typeof ssti !== "number" && ssti !== null)) {
        return null;
      }
      if (src.bytes !== undefined) {
        let bytesArray: number[];
        if (Array.isArray(src.bytes)) {
          bytesArray = src.bytes;
        } else if (typeof src.bytes === "object" && src.bytes !== null) {
          const values = Object.values(src.bytes);
          if (!values.every((b: unknown) => typeof b === "number" && b >= 0 && b <= 255)) {
            return null;
          }
          bytesArray = values as number[];
        } else {
          return null;
        }
        if (!bytesArray.every((b: number) => b >= 0 && b <= 255) || bytesArray.length > P2P_MAX_TORRENT_BYTES) {
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
    case "chat":
      if (typeof message.content === "string" && message.content.trim().length > 0) {
        return { type: "chat", content: message.content.trim() };
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
  | "destroyed"
  | "reconnecting";

const VALID_TRANSITIONS: Record<P2PServiceState, ReadonlySet<P2PServiceState>> = {
  disconnected: new Set(["connecting", "destroyed"]),
  connecting: new Set(["connected", "disconnected", "destroyed"]),
  connected: new Set(["disconnecting", "disconnected", "reconnecting", "destroyed"]),
  disconnecting: new Set(["disconnected", "destroyed"]),
  reconnecting: new Set(["connected", "disconnected", "destroyed"]),
  destroyed: new Set(),
};

function canTransition(from: P2PServiceState, to: P2PServiceState): boolean {
  if (from === to) return true;
  return VALID_TRANSITIONS[from]?.has(to) ?? false;
}

export class P2PService {
  private peerId: string;
  private role: "host" | "guest" = "host";
  private peer: Peer | null = null;
  private connections = new Map<string, DataConnection>();
  private connectionCleanups = new Map<string, Array<() => void>>();
  private remotePeerId: string | null = null;
  private _state: P2PServiceState = "disconnected";
  private setState(next: P2PServiceState): void {
    if (next === this._state) return;
    if (!canTransition(this._state, next)) {
      p2pLogger.warn(`Invalid state transition: ${this._state} -> ${next}`);
      return;
    }
    p2pLogger.debug(`State: ${this._state} -> ${next}`);
    this._state = next;
  }
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectStartTime = 0;
  private maxReconnectWindowMs = 120_000;
  private isReconnecting = false;
  private isDisconnecting = false;
  private lastRttMs: number | null = null;
  private pingIntervalId: ReturnType<typeof setInterval> | null = null;
  private reconnectTimeoutId: ReturnType<typeof setTimeout> | null = null;
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
    chat_received: new Set(),
    resend_requested: new Set(),
    reconnect_failed: new Set(),
  };

  constructor() {
    this.peerId = generatePeerId();
    this.cleanup = createCleanup();
  }

  /** Returns the current peer ID. */
  getPeerId(): string {
    return this.peerId;
  }

  /** Returns the last measured round-trip time in milliseconds, or null if not yet measured. */
  getLastRttMs(): number | null {
    return this.lastRttMs;
  }

  /** Returns the current connection quality based on RTT measurements. */
  getConnectionQuality(): ConnectionQuality {
    if (this.lastRttMs === null) return "unknown";
    if (this.lastRttMs < 100) return "good";
    if (this.lastRttMs < 300) return "fair";
    return "poor";
  }

  /** Set this peer as the room host. */
  setHost(): void {
    this.role = "host";
  }

  /** Set this peer as a guest. */
  setGuest(): void {
    this.role = "guest";
  }

  /** Returns true if this peer is the host. */
  isHost(): boolean {
    return this.role === "host";
  }

  /** Returns the current connection state machine state. */
  getState(): P2PServiceState {
    return this._state;
  }

  /** Returns true if at least one data connection is open. */
  isConnected(): boolean {
    return this.getOpenConnectionCount() > 0;
  }

  /** Returns true if the peer is currently in a room (initialized and not disconnected/destroyed). */
  isInRoom(): boolean {
    return this.peer !== null && this._state !== "disconnected" && this._state !== "destroyed";
  }

  /**
   * Register an event listener.
   * @returns An unsubscribe function.
   */
   on<K extends EventKey>(event: K, callback: P2PEvents[K]): () => void {
     this.listeners[event].add(callback);
     return () => this.listeners[event].delete(callback);
   }

  /**
   * Connect to a remote peer by their full PeerJS ID.
   * Retries up to P2P_CONFIG.connectRetryAttempts times with exponential backoff.
   * @throws If the service is destroyed, already connecting, or all retries fail.
   */
  async connect(remotePeerId: string): Promise<void> {
    if (this._state === "connecting") {
      throw new Error("Connection already in progress");
    }
    if (this._state === "destroyed") {
      throw new Error("Cannot connect: P2PService has been destroyed and cannot be reused");
    }
    const existing = this.getConnection(remotePeerId);
    if (existing?.open) return;

    const maxRetries = P2P_CONFIG.connectRetryAttempts;
    const baseDelay = P2P_CONFIG.connectRetryBaseDelayMs;
    let lastError: Error | null = null;

    let failed = false;
    for (let attempt = 1; attempt <= maxRetries; attempt++) {
      try {
        await this.tryConnect(remotePeerId);
        failed = false;
        return;
      } catch (error) {
        lastError = error instanceof Error ? error : new Error(String(error));
        p2pLogger.warn(`Connection attempt ${attempt}/${maxRetries} failed: ${lastError.message}`);
        failed = true;
        if (attempt < maxRetries) {
          const delay = baseDelay * Math.pow(2, attempt - 1);
          const jitteredDelay = delay * (0.5 + Math.random() * 0.5);
          await new Promise<void>((resolve) => { setTimeout(resolve, jitteredDelay); });
        }
      }
    }

    if (failed && this._state !== "connected") {
      this.setState("disconnected");
    }

    throw lastError ?? new Error("Connection failed after retries");
  }

  private async tryConnect(remotePeerId: string): Promise<void> {
    if (!this.peer) {
      throw new Error("Peer not initialized — call initialize() first");
    }
    this.remotePeerId = remotePeerId;
    this.setState("connecting");

    try {
      return await new Promise((resolve, reject) => {
        const conn = this.peer!.connect(remotePeerId, {
          reliable: true,
          serialization: "json",
        });

        let isSettled = false;
        let timeoutId: ReturnType<typeof setTimeout> | null = null;

        const settleResolve = () => {
          if (isSettled) return;
          isSettled = true;
          this.setState("connected");
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
          this.setState("disconnected");
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
      this.setState("disconnected");
      throw error;
    }
  }

  /**
   * Initialize the PeerJS connection and register event handlers.
   * Must be called before connect() or sending messages.
   * @throws If WebRTC is unavailable or initialization times out.
   */
  initialize(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.peer) {
        resolve();
        return;
      }
      if (this._state === "destroyed") {
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
        if (this._state === "disconnecting") return;
        this.setState("disconnected");
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
    if (this.isDisconnecting) {
      this.isReconnecting = false;
      return;
    }
    if (this._state === "destroyed" || this._state === "disconnecting") return;
    if (this.role === "host") return;
    if (!this.remotePeerId) return;
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      p2pLogger.warn("Max reconnect attempts reached, giving up");
      this.isReconnecting = false;
      this.isDisconnecting = false;
      this.reconnectAttempts = 0;
      this.setState("disconnected");
      this.emit("error", new Error("Connection lost. Please rejoin the room."));
      this.emit("reconnect_failed");
      return;
    }
    if (this.reconnectAttempts === 0) {
      this.reconnectStartTime = Date.now();
    }
    if (Date.now() - this.reconnectStartTime > this.maxReconnectWindowMs) {
      p2pLogger.warn("Max reconnect time window exceeded, giving up");
      this.isReconnecting = false;
      this.isDisconnecting = false;
      this.reconnectAttempts = 0;
      this.setState("disconnected");
      this.emit("error", new Error("Connection lost. Please rejoin the room."));
      this.emit("reconnect_failed");
      return;
    }
    if (this.isReconnecting) return;

    this.isReconnecting = true;
    this.setState("reconnecting");
    this.reconnectAttempts++;
    const baseReconnectDelay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), 30_000);
    const delay = baseReconnectDelay * (0.5 + Math.random() * 0.5);
    p2pLogger.info(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
    this.emit("reconnecting", this.reconnectAttempts, delay);

    this.reconnectTimeoutId = this.cleanup.setTimeout(async () => {
      this.reconnectTimeoutId = null;
      if (this.isDisconnecting) {
        this.isReconnecting = false;
        this.setState("disconnected");
        return;
      }
      if (this._state === "destroyed" || this._state === "disconnecting") {
        this.isReconnecting = false;
        this.isDisconnecting = false;
        this.setState("disconnected");
        return;
      }
      if (this.isReconnecting && this._state === "connected") {
        return;
      }
      try {
        if (this.peer?.destroyed) {
          this.peer.reconnect();
        }
        if (this.remotePeerId) {
          await this.connect(this.remotePeerId);
        }
        this.reconnectAttempts = 0;
        this.isReconnecting = false;
        this.isDisconnecting = false;
        this.setState("connected");
      } catch (error) {
        p2pLogger.warn("Reconnect failed:", error);
        this.isReconnecting = false;
        if (this.reconnectTimeoutId === null) {
          try {
            this.attemptReconnect();
          } catch (reconnectError) {
            p2pLogger.error("Reconnect attempt failed unexpectedly:", reconnectError);
            this.isReconnecting = false;
            this.setState("disconnected");
            this.emit("error", new Error("Connection lost. Please rejoin the room."));
            this.emit("reconnect_failed");
          }
        }
      }
    }, delay);
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
      const oldCleanups = this.connectionCleanups.get(peerId);
      if (oldCleanups) {
        for (const cleanup of oldCleanups) cleanup();
        this.connectionCleanups.delete(peerId);
      }
      existingConnection.close();
    }

    if (this._state === "disconnecting") return;

    this.connections.set(peerId, conn);
    const cleanups: Array<() => void> = [];

    const wasAlreadyOpen = conn.open;

    const onConnOpen = () => {
      if (this.connections.get(peerId) !== conn) return;
      if (this._state === "disconnecting") return;
      if (options.emitPeerConnected) this.emit("peer_connected", peerId);
      options.onOpen?.();
      this.ensurePingInterval();
    };

    if (wasAlreadyOpen) {
      if (options.emitPeerConnected) this.emit("peer_connected", peerId);
      options.onOpen?.();
      this.ensurePingInterval();
    } else {
      conn.on("open", onConnOpen);
      cleanups.push(() => { conn.off?.("open", onConnOpen); });
    }

    const onConnData = (data: unknown) => {
      if (this.connections.get(peerId) !== conn) return;
      const message = parseInboundMessage(data);
      if (!message) return;
      if (message.type === "sync" && this.isSyncRateLimited(peerId)) {
        p2pLogger.warn(`Sync rate limit exceeded for peer ${peerId}, dropping message`);
        return;
      }
      if (message.type === "chat" && this.isChatRateLimited(peerId)) {
        p2pLogger.warn(`Chat rate limit exceeded for peer ${peerId}, dropping message`);
        return;
      }
      if (message.type === "torrent_source" && this.isTorrentSourceRateLimited(peerId)) {
        p2pLogger.warn(`Torrent source rate limit exceeded for peer ${peerId}, dropping message`);
        return;
      }
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
           });
           break;
        case "ping":
          // Respond with pong, echoing the timestamp
          this.sendPayload({ type: "pong", ts: message.ts });
          break;
        case "pong":
          this.handlePong(message.ts);
          break;
        case "chat":
          this.emit("chat_received", conn.peer, message.content);
          if (this.role === "host") {
            for (const [peerId, connection] of this.connections.entries()) {
              if (peerId !== conn.peer && connection.open) {
                connection.send({ type: "chat", content: message.content });
              }
            }
            if (message.content.trim().toLowerCase() === "/resend") {
              this.emit("resend_requested", conn.peer);
            }
          }
          break;
      }
    };
    conn.on("data", onConnData);
    cleanups.push(() => { conn.off?.("data", onConnData); });

    const onConnClose = () => {
      const trackedConnection = this.connections.get(peerId);
      if (trackedConnection === conn) {
        this.connections.delete(peerId);
      }
      if (this.remotePeerId === peerId) {
        this.remotePeerId = null;
      }
      if (this._state === "disconnecting") return;
      this.emit("peer_disconnected", peerId);
      options.onClose?.();
      if (this.getOpenConnectionCount() === 0) {
        this.stopPingInterval();
        this.emit("disconnected");
      }
    };
    conn.on("close", onConnClose);
    cleanups.push(() => { conn.off?.("close", onConnClose); });

    const onConnError = (err: Error) => {
      if (this.connections.get(peerId) !== conn) return;
      this.emit("error", err instanceof Error ? err : new Error(String(err)));
    };
    conn.on("error", onConnError);
    cleanups.push(() => { conn.off?.("error", onConnError); });

    this.connectionCleanups.set(peerId, cleanups);
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

  private ensurePingInterval(): void {
    if (this.pingIntervalId !== null) return;
    this.pingIntervalId = this.cleanup.setInterval(() => {
      this.sendPing();
    }, 2000);
    this.startRateLimitCleanup();
  }

  private stopPingInterval(): void {
    if (this.pingIntervalId !== null) {
      clearInterval(this.pingIntervalId);
      this.pingIntervalId = null;
    }
  }

  private startRateLimitCleanup(): void {
    if (this.rateLimitCleanupTimer !== null) return;
    this.rateLimitCleanupTimer = this.cleanup.setInterval(() => {
      const now = Date.now();
      for (const [peerId, timestamps] of this.syncMessageTimestamps) {
        const recent = timestamps.filter((ts) => now - ts < this.syncRateLimitWindowMs);
        if (recent.length === 0) {
          this.syncMessageTimestamps.delete(peerId);
        } else {
          this.syncMessageTimestamps.set(peerId, recent);
        }
      }
      for (const [peerId, timestamps] of this.chatMessageTimestamps) {
        const recent = timestamps.filter((ts) => now - ts < this.chatRateLimitWindowMs);
        if (recent.length === 0) {
          this.chatMessageTimestamps.delete(peerId);
        } else {
          this.chatMessageTimestamps.set(peerId, recent);
        }
      }
    }, this.rateLimitCleanupIntervalMs);
  }

  /** Clear rate limit tracking data for a specific peer. */
  clearRateLimitForPeer(peerId: string): void {
    this.syncMessageTimestamps.delete(peerId);
    this.chatMessageTimestamps.delete(peerId);
    this.torrentSourceMessageTimestamps.delete(peerId);
  }

  private syncMessageTimestamps = new Map<string, number[]>();
  private readonly syncRateLimit = 10;
  private readonly syncRateLimitWindowMs = 1000;
  private readonly maxSyncTimestampsPerPeer = 50;
  private chatMessageTimestamps = new Map<string, number[]>();
  private readonly chatRateLimit = 10;
  private readonly chatRateLimitWindowMs = 10_000;
  private readonly maxChatTimestampsPerPeer = 50;
  private torrentSourceMessageTimestamps = new Map<string, number[]>();
  private readonly torrentSourceRateLimit = 1;
  private readonly torrentSourceRateLimitWindowMs = 3000;
  private readonly maxTorrentSourceTimestampsPerPeer = 10;
  private rateLimitCleanupTimer: ReturnType<typeof setInterval> | null = null;
  private readonly rateLimitCleanupIntervalMs = 60_000;

  private isSyncRateLimited(peerId: string): boolean {
    const now = Date.now();
    const timestamps = this.syncMessageTimestamps.get(peerId) ?? [];
    const recent = timestamps.filter((ts) => now - ts < this.syncRateLimitWindowMs);
    recent.push(now);
    if (recent.length > this.maxSyncTimestampsPerPeer) {
      recent.splice(0, recent.length - this.maxSyncTimestampsPerPeer);
    }
    this.syncMessageTimestamps.set(peerId, recent);
    return recent.length > this.syncRateLimit;
  }

  private isChatRateLimited(peerId: string): boolean {
    const now = Date.now();
    const timestamps = this.chatMessageTimestamps.get(peerId) ?? [];
    const recent = timestamps.filter((ts) => now - ts < this.chatRateLimitWindowMs);
    recent.push(now);
    if (recent.length > this.maxChatTimestampsPerPeer) {
      recent.splice(0, recent.length - this.maxChatTimestampsPerPeer);
    }
    this.chatMessageTimestamps.set(peerId, recent);
    return recent.length > this.chatRateLimit;
  }

  private isTorrentSourceRateLimited(peerId: string): boolean {
    const now = Date.now();
    const timestamps = this.torrentSourceMessageTimestamps.get(peerId) ?? [];
    const recent = timestamps.filter((ts) => now - ts < this.torrentSourceRateLimitWindowMs);
    recent.push(now);
    if (recent.length > this.maxTorrentSourceTimestampsPerPeer) {
      recent.splice(0, recent.length - this.maxTorrentSourceTimestampsPerPeer);
    }
    this.torrentSourceMessageTimestamps.set(peerId, recent);
    return recent.length > this.torrentSourceRateLimit;
  }

  private readonly pendingPings = new Map<number, number>();
  private readonly maxPendingPings = 10;

  private sendPing(): void {
    if (!this.isConnected()) return;
    const now = Date.now();
    if (this.pendingPings.size >= this.maxPendingPings) {
      const oldest = this.pendingPings.keys().next().value;
      if (oldest !== undefined) this.pendingPings.delete(oldest);
    }
    this.pendingPings.set(now, now);
    const pingPayload = { type: "ping", ts: now };
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
    const now = Date.now();
    if (!this.pendingPings.has(pongTs)) return;
    this.pendingPings.delete(pongTs);
    if (pongTs > now || pongTs < now - 5000) return;
    const rtt = now - pongTs;
    if (rtt > 0) {
      this.lastRttMs = rtt;
      this.emit("connection_quality", this.getConnectionQuality());
    }
  }

    /**
     * Send a torrent source to connected peer(s).
     * @param source - The shared torrent source (magnet or file).
     * @param selectedMediaIndex - Optional index of the selected media file.
     * @param selectedAudioTrackIndex - Optional index of the selected audio track.
     * @param selectedSubtitleIndex - Optional index of the selected subtitle track.
     * @param targetPeerId - Optional specific peer to send to; broadcasts if omitted.
     */
    public sendTorrentSource(source: SharedTorrentSource, selectedMediaIndex: number | null, selectedAudioTrackIndex: number | null, selectedSubtitleIndex: number | null, targetPeerId?: string): void {
      if (selectedMediaIndex !== null && (!Number.isFinite(selectedMediaIndex) || selectedMediaIndex < 0)) return;
      if (selectedAudioTrackIndex !== null && (!Number.isFinite(selectedAudioTrackIndex) || selectedAudioTrackIndex < 0)) return;
      if (selectedSubtitleIndex !== null && (!Number.isFinite(selectedSubtitleIndex) || selectedSubtitleIndex < 0)) return;
      if (source.kind === "magnet" && source.magnetLink.length > 8000) {
        p2pLogger.warn("Magnet link too long, dropping");
        return;
      }
      if (source.kind === "file" && source.bytes.length > P2P_MAX_TORRENT_BYTES) {
        const errMsg = `Torrent file too large (${(source.bytes.length / 1024 / 1024).toFixed(1)} MB). Maximum size is ${P2P_MAX_TORRENT_BYTES / 1024 / 1024} MB.`;
        p2pLogger.warn(errMsg);
        this.emit("error", new Error(errMsg));
        return;
      }
      this.sendPayload({
        type: "torrent_source",
        source,
        selectedMediaIndex,
        selectedAudioTrackIndex,
        selectedSubtitleIndex,
      }, targetPeerId);
    }

     /**
      * Send room configuration (sync tolerance) to connected peer(s).
      * @param syncToleranceSeconds - The sync tolerance in seconds (clamped to 0-30).
      * @param targetPeerId - Optional specific peer to send to; broadcasts if omitted.
      */
     public sendRoomConfig(syncToleranceSeconds: number, targetPeerId?: string): void {
       if (!Number.isFinite(syncToleranceSeconds) || syncToleranceSeconds < 0) return;
       const clampedTolerance = Math.min(syncToleranceSeconds, 30);
       this.sendPayload({
         type: "room_config",
         syncToleranceSeconds: clampedTolerance,
       }, targetPeerId);
     }

  /**
   * Send a sync message to connected peer(s).
   * @param message - The sync message to send.
   * @param targetPeerId - Optional specific peer to send to; broadcasts if omitted.
   */
  public sendSync(message: SyncMessage, targetPeerId?: string): void {
    if (!message || !Number.isFinite(message.position) || !Number.isFinite(message.server_ts)) return;
    this.sendPayload({
      type: "sync",
      message,
    }, targetPeerId);
  }

  /**
   * Send a chat message to connected peer(s).
   * @param content - The message content to send.
   * @param targetPeerId - Optional specific peer to send to; broadcasts if omitted.
   */
  public sendChat(content: string, targetPeerId?: string): void {
    if (!content?.trim()) return;
    this.sendPayload({ type: "chat", content }, targetPeerId);
  }

  /** Disconnect from all peers and clean up resources. Idempotent. */
  public async disconnect(): Promise<void> {
    if (this.isDisconnecting) return;
    this.isDisconnecting = true;
    this.isReconnecting = false;
    if (this.reconnectTimeoutId !== null) {
      clearTimeout(this.reconnectTimeoutId);
      this.reconnectTimeoutId = null;
    }
    this.stopPingInterval();
    for (const cleanups of this.connectionCleanups.values()) {
      for (const cleanup of cleanups) cleanup();
    }
    this.connectionCleanups.clear();
    this.connections.forEach((conn) => conn.close());
    this.connections.clear();
    this.syncMessageTimestamps.clear();
    this.chatMessageTimestamps.clear();
    this.torrentSourceMessageTimestamps.clear();
    this.pendingPings.clear();
    if (this.rateLimitCleanupTimer !== null) {
      clearInterval(this.rateLimitCleanupTimer);
      this.rateLimitCleanupTimer = null;
    }
    if (this.peer && !this.peer.destroyed) {
      try {
        this.peer.disconnect();
      } catch (error) {
        p2pLogger.warn("peer.disconnect() failed during disconnect():", error);
      }
      if (typeof this.peer.destroy === "function") {
        this.peer.destroy();
      }
    }
    this.peer = null;
    this.remotePeerId = null;
    this.cleanup.abort();
    this.setState("disconnected");
    this.emit("disconnected");
  }
 }
 
export default P2PService;
