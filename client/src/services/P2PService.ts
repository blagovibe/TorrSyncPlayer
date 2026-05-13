import { Peer, type DataConnection, type PeerJSOption } from "peerjs";
import {
  type RoomConfigMessage,
  type SharedTorrentSource,
  type SyncMessage,
} from "./types";

type TorrentSourceMessage = {
  source: SharedTorrentSource;
  selectedMediaIndex: number | null;
  selectedAudioTrackIndex: number | null;
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
};

type EventKey = keyof P2PEvents;

type OutboundMessage =
  | { type: "sync"; message: SyncMessage }
  | {
      type: "torrent_source";
      source: SharedTorrentSource;
      selectedMediaIndex: number | null;
      selectedAudioTrackIndex: number | null;
    }
  | { type: "room_config"; syncToleranceSeconds: number };

const PEER_ID_LENGTH = 6;
const PEER_ID_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
const DEFAULT_PEERJS_HOST = "0.peerjs.com";
const DEFAULT_PEERJS_PORT = 443;
const DEFAULT_PEERJS_PATH = "/";
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
  const bytes = new Uint8Array(PEER_ID_LENGTH);
  crypto.getRandomValues(bytes);
  let id = "";
  for (let i = 0; i < PEER_ID_LENGTH; i++) {
    id += PEER_ID_CHARS.charAt(bytes[i] % PEER_ID_CHARS.length);
  }
  return id;
}

function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
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
  let normalizedPath = path.trim() || DEFAULT_PEERJS_PATH;
  if (!normalizedPath.startsWith("/")) {
    normalizedPath = `/${normalizedPath}`;
  }
  if (!normalizedPath.endsWith("/")) {
    normalizedPath = `${normalizedPath}/`;
  }
  return normalizedPath;
}

function parsePeerPort(port: string | undefined): number {
  if (!port?.trim()) {
    return DEFAULT_PEERJS_PORT;
  }

  const parsedPort = Number(port);
  if (!Number.isInteger(parsedPort) || parsedPort <= 0 || parsedPort > 65535) {
    return DEFAULT_PEERJS_PORT;
  }
  return parsedPort;
}

function parsePeerSecure(secure: string | undefined, host: string): boolean {
  if (secure === "false" || secure === "0") {
    return false;
  }
  if (secure === "true" || secure === "1") {
    return true;
  }
  return host === DEFAULT_PEERJS_HOST || location.protocol === "https:";
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

  if (!candidate || typeof candidate !== "object") {
    return null;
  }

  const message = candidate as Partial<OutboundMessage> & { type?: string };

  switch (message.type) {
    case "sync":
      if (message.message && isSyncMessage(message.message)) {
        return { type: "sync", message: message.message };
      }
      return null;
    case "torrent_source":
      if (
        message.source &&
        (typeof message.selectedMediaIndex === "number" || message.selectedMediaIndex === null) &&
        (typeof message.selectedAudioTrackIndex === "number" ||
          message.selectedAudioTrackIndex === null)
      ) {
        return {
          type: "torrent_source",
          source: message.source as SharedTorrentSource,
          selectedMediaIndex: message.selectedMediaIndex,
          selectedAudioTrackIndex: message.selectedAudioTrackIndex,
        };
      }
      return null;
    case "room_config":
      if (typeof message.syncToleranceSeconds === "number" && Number.isFinite(message.syncToleranceSeconds)) {
        return {
          type: "room_config",
          syncToleranceSeconds: message.syncToleranceSeconds,
        };
      }
      return null;
    default:
      return null;
  }
}

export function buildPeerServerOptions(env: PeerServerEnv): PeerJSOption {
  const host = env.VITE_PEERJS_HOST?.trim() || DEFAULT_PEERJS_HOST;

  return {
    host,
    port: parsePeerPort(env.VITE_PEERJS_PORT),
    path: normalizePeerPath(env.VITE_PEERJS_PATH ?? DEFAULT_PEERJS_PATH),
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

  if (
    peerError.type === "browser-incompatible" ||
    message.includes("The current browser does not support WebRTC")
  ) {
    return createWebRTCUnavailableError("PeerJS could not initialize WebRTC data channels");
  }

  if (peerError.type === "network" || message.includes("Lost connection to server")) {
    return createSignalingUnavailableError(message);
  }

  return error instanceof Error ? error : new Error(message);
}

export class P2PService {
  private peerId: string;
  private role: "host" | "guest" = "host";
  private peer: Peer | null = null;
  private connections = new Map<string, DataConnection>();
  private remotePeerId: string | null = null;
  private isConnecting = false;
  private isDisconnecting = false;
  private connectTimeoutId: number | null = null;
  private listeners: { [K in EventKey]: Set<P2PEvents[K]> } = {
    connected: new Set(),
    disconnected: new Set(),
    peer_connected: new Set(),
    peer_disconnected: new Set(),
    sync: new Set(),
    torrent_source: new Set(),
    room_config: new Set(),
    error: new Set(),
  };

  constructor() {
    this.peerId = generatePeerId();
  }

  getPeerId(): string {
    return this.peerId;
  }

  setHost(): void {
    this.role = "host";
  }

  setGuest(): void {
    this.role = "guest";
  }

  isHost(): boolean {
    return this.role === "host";
  }

  on<K extends EventKey>(event: K, callback: P2PEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  async connect(remotePeerId: string): Promise<void> {
    if (this.isConnecting || this.getConnection(remotePeerId)?.open) {
      return;
    }

    this.remotePeerId = remotePeerId;
    this.isConnecting = true;

    return new Promise((resolve, reject) => {
      if (!this.peer) {
        this.isConnecting = false;
        reject(new Error("Peer not initialized"));
        return;
      }

      if (this.connectTimeoutId !== null) {
        window.clearTimeout(this.connectTimeoutId);
        this.connectTimeoutId = null;
      }

      const conn = this.peer.connect(remotePeerId, {
        reliable: true,
        serialization: "json",
      });

      let isSettled = false;
      const settleResolve = () => {
        if (isSettled) return;
        isSettled = true;
        if (this.connectTimeoutId !== null) {
          window.clearTimeout(this.connectTimeoutId);
        }
        this.isConnecting = false;
        this.connectTimeoutId = null;
        if (this.getOpenConnectionCount() === 1) {
          this.emit("connected");
        }
        resolve();
      };
      const settleReject = (error: Error) => {
        if (isSettled) return;
        isSettled = true;
        if (this.connectTimeoutId !== null) {
          window.clearTimeout(this.connectTimeoutId);
        }
        this.isConnecting = false;
        this.connectTimeoutId = null;
        reject(error);
      };

      this.bindConnection(remotePeerId, conn, {
        emitPeerConnected: true,
        onOpen: settleResolve,
        onClose: () => settleReject(new Error("Connection closed before it was established")),
      });

      this.connectTimeoutId = window.setTimeout(() => {
        if (!isSettled && this.isConnecting) {
          conn.close();
          settleReject(new Error("Connection timeout"));
        }
      }, 30000);
    });
  }

  initialize(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.peer) {
        resolve();
        return;
      }

      const webRTCSupportIssue = getWebRTCSupportIssue();
      if (webRTCSupportIssue) {
        reject(createWebRTCUnavailableError(webRTCSupportIssue));
        return;
      }

      let isSettled = false;
      let initTimeoutId: number | null = null;
      const settleResolve = () => {
        if (isSettled) return;
        isSettled = true;
        if (initTimeoutId !== null) {
          window.clearTimeout(initTimeoutId);
          initTimeoutId = null;
        }
        resolve();
      };
      const settleReject = (error: Error) => {
        if (isSettled) return;
        isSettled = true;
        if (initTimeoutId !== null) {
          window.clearTimeout(initTimeoutId);
          initTimeoutId = null;
        }
        reject(error);
      };

      const peerServerOptions = getPeerServerOptions();
      if (this.role === "host") {
        this.peer = new Peer(`torrsync-${this.peerId}`, {
          ...peerServerOptions,
        });
      } else {
        this.peer = new Peer({
          ...peerServerOptions,
        });
      }

      this.peer.on("open", (id) => {
        console.log("PeerJS initialized with ID:", id);
        if (id) {
          this.peerId = id.replace("torrsync-", "");
        }
        settleResolve();
      });

      this.peer.on("connection", (conn) => {
        this.handleIncomingConnection(conn);
      });

      this.peer.on("error", (err) => {
        const error = normalizePeerError(err);
        console.error("PeerJS error:", err);
        this.emit("error", error);
        if (err.type === "peer-unavailable") {
          settleReject(new Error("Peer not found"));
        } else {
          settleReject(error);
        }
      });

      this.peer.on("disconnected", () => {
        if (initTimeoutId !== null) {
          window.clearTimeout(initTimeoutId);
          initTimeoutId = null;
        }
        this.emit("disconnected");
      });

      initTimeoutId = window.setTimeout(() => {
        if (!this.peer?.open) {
          settleReject(new Error("Initialization timeout"));
        }
      }, 15000);
    });
  }

  sendSync(message: SyncMessage, targetPeerId?: string): void {
    this.sendPayload({ type: "sync", message }, targetPeerId);
  }

  sendTorrentSource(
    payload: {
      source: SharedTorrentSource;
      selectedMediaIndex: number | null;
      selectedAudioTrackIndex: number | null;
    },
    targetPeerId?: string,
  ): void {
    this.sendPayload({ type: "torrent_source", ...payload }, targetPeerId);
  }

  sendRoomConfig(payload: RoomConfigMessage, targetPeerId?: string): void {
    this.sendPayload({ type: "room_config", syncToleranceSeconds: payload.syncToleranceSeconds }, targetPeerId);
  }

  disconnect(): void {
    this.isDisconnecting = true;

    if (this.connectTimeoutId !== null) {
      window.clearTimeout(this.connectTimeoutId);
      this.connectTimeoutId = null;
    }

    for (const connection of this.connections.values()) {
      connection.close();
    }
    this.connections.clear();

    if (this.peer) {
      this.peer.destroy();
      this.peer = null;
    }

    this.remotePeerId = null;
    this.isConnecting = false;
    this.isDisconnecting = false;
    this.emit("disconnected");
  }

  isConnected(): boolean {
    return this.getOpenConnectionCount() > 0;
  }

  getRemotePeerId(): string | null {
    return this.remotePeerId ?? this.connections.keys().next().value ?? null;
  }

  private handleIncomingConnection(conn: DataConnection): void {
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

    conn.on("open", () => {
      if (this.connections.get(peerId) !== conn) {
        return;
      }

      if (this.isDisconnecting) {
        return;
      }

      if (options.emitPeerConnected) {
        this.emit("peer_connected", peerId);
      }
      options.onOpen?.();
    });

    conn.on("data", (data) => {
      if (this.connections.get(peerId) !== conn) {
        return;
      }

      const message = parseInboundMessage(data);
      if (!message) {
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
          });
          break;
        case "room_config":
          this.emit("room_config", {
            syncToleranceSeconds: message.syncToleranceSeconds,
          });
          break;
      }
    });

    conn.on("close", () => {
      const trackedConnection = this.connections.get(peerId);
      if (trackedConnection === conn) {
        this.connections.delete(peerId);
      }

      if (this.remotePeerId === peerId) {
        this.remotePeerId = null;
      }

      if (this.isDisconnecting) {
        return;
      }

      this.emit("peer_disconnected", peerId);
      options.onClose?.();

      if (this.getOpenConnectionCount() === 0) {
        this.emit("disconnected");
      }
    });

    conn.on("error", (err) => {
      if (this.connections.get(peerId) !== conn) {
        return;
      }

      this.emit("error", err instanceof Error ? err : new Error(String(err)));
    });
  }

  private sendPayload(payload: OutboundMessage, targetPeerId?: string): void {
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
      if (connection.open) {
        count += 1;
      }
    }
    return count;
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<P2PEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<P2PEvents[K]>) => void)(...args);
    }
  }
}

export default P2PService;
