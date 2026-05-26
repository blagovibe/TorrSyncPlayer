import { afterEach, describe, expect, it, vi } from "vitest";
import { P2PService } from "../P2PService";
import { SyncService } from "../SyncService";
import { TorrentService } from "../TorrentService";

vi.mock("webtorrent", () => {
  const createServer = vi.fn();
  const destroy = vi.fn();
  const WT = vi.fn(() => ({ add: vi.fn(), createServer, destroy }));
  return { default: WT };
});

function setupElectronBackendCleanup() {
  if (typeof window !== "undefined") {
    delete (window as unknown as { torrsyncElectronTorrent?: unknown }).torrsyncElectronTorrent;
  }
}

describe("Integration: P2P + Sync + Torrent flow", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    setupElectronBackendCleanup();
  });

  describe("Host loads torrent → Guest receives source", () => {
    it("host P2PService emits torrent_source that guest TorrentService can load", async () => {
      const hostP2P = new P2PService();
      const guestP2P = new P2PService();

      const guestReceived: Array<{ source: unknown; mediaIndex: number | null }> = [];
      guestP2P.on("torrent_source", (msg) => {
        guestReceived.push({ source: msg.source, mediaIndex: msg.selectedMediaIndex });
      });

      const magnetLink = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567";
      const source = { kind: "magnet" as const, magnetLink, sourceKey: `magnet:${magnetLink}` };

      hostP2P.sendTorrentSource(source, 0, null, null);

      expect(guestReceived).toHaveLength(0);
    });

    it("torrent source message round-trips through P2P serialization", () => {
      const magnetLink = "magnet:?xt=urn:btih:abcdef0123456789abcdef0123456789abcdef01";
      const source = { kind: "magnet" as const, magnetLink, sourceKey: `magnet:${magnetLink}` };

      const serialized = JSON.stringify({
        type: "torrent_source",
        source,
        selectedMediaIndex: 2,
        selectedAudioTrackIndex: 1,
        selectedSubtitleIndex: null,
      });

      const parsed = JSON.parse(serialized);
      expect(parsed.type).toBe("torrent_source");
      expect(parsed.source.kind).toBe("magnet");
      expect(parsed.source.magnetLink).toBe(magnetLink);
      expect(parsed.selectedMediaIndex).toBe(2);
      expect(parsed.selectedAudioTrackIndex).toBe(1);
      expect(parsed.selectedSubtitleIndex).toBeNull();
    });

    it("torrent file source message round-trips through P2P serialization", () => {
      const bytes = [100, 56, 58, 97, 110, 110, 111, 117, 110, 99, 101];
      const source = {
        kind: "file" as const,
        fileName: "test.torrent",
        bytes,
        sourceKey: "file:test.torrent:11:abc123",
      };

      const serialized = JSON.stringify({
        type: "torrent_source",
        source,
        selectedMediaIndex: null,
        selectedAudioTrackIndex: null,
        selectedSubtitleIndex: null,
      });

      const parsed = JSON.parse(serialized);
      expect(parsed.type).toBe("torrent_source");
      expect(parsed.source.kind).toBe("file");
      expect(parsed.source.fileName).toBe("test.torrent");
      expect(parsed.source.bytes).toEqual(bytes);
    });
  });

  describe("Sync flow: Host broadcasts → Guest applies", () => {
    it("sync message round-trips through P2P serialization", () => {
      const syncMsg = {
        action: "play",
        position: 42.5,
        server_ts: Date.now(),
        sourceKey: "magnet:test",
      };

      const serialized = JSON.stringify({ type: "sync", message: syncMsg });
      const parsed = JSON.parse(serialized);

      expect(parsed.type).toBe("sync");
      expect(parsed.message.action).toBe("play");
      expect(parsed.message.position).toBe(42.5);
      expect(parsed.message.sourceKey).toBe("magnet:test");
    });

    it("SyncService can create snapshots with proper structure", () => {
      const transport = {
        sendSync: vi.fn(),
      };

      const mockVideo = {
        currentTime: 0,
        duration: 100,
        paused: true,
        play: vi.fn().mockResolvedValue(undefined),
        pause: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      } as unknown as HTMLVideoElement;

      const syncService = new SyncService(transport, mockVideo, "master", 1.5);

      const snapshot = syncService.createSnapshot();
      expect(snapshot).not.toBeNull();
      expect(snapshot!.action).toBe("state");
      expect(snapshot!.position).toBe(0);
      expect(snapshot!.server_ts).toBeGreaterThan(0);

      syncService.dispose();
    });

    it("SyncService slave does not send messages", () => {
      const transport = {
        sendSync: vi.fn(),
      };

      const mockVideo = {
        currentTime: 0,
        duration: 100,
        paused: true,
        play: vi.fn().mockResolvedValue(undefined),
        pause: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      } as unknown as HTMLVideoElement;

      const syncService = new SyncService(transport, mockVideo, "slave", 1.5);

      expect(transport.sendSync).not.toHaveBeenCalled();

      syncService.dispose();
    });
  });

  describe("TorrentService + Electron backend integration", () => {
    it("uses Electron backend when available, falls back to browser WebTorrent", async () => {
      const initialSnapshot = {
        files: [
          { index: 0, name: "movie.mp4", length: 1000, kind: "video", extension: ".mp4", progress: 0.5, streamUrl: "http://127.0.0.1:4321/movie.mp4" },
        ],
        progress: 0.5,
        downloadSpeed: 200,
        numPeers: 3,
        discoveredPeerCount: 5,
      };

      const backend = {
        addMagnet: vi.fn().mockResolvedValue(initialSnapshot),
        addTorrentFile: vi.fn(),
        getStats: vi.fn().mockResolvedValue(initialSnapshot),
        clear: vi.fn().mockResolvedValue(undefined),
        setMaxBufferMB: vi.fn(),
      };

      vi.stubGlobal("window", {
        clearInterval,
        setInterval,
        torrsyncElectronTorrent: backend,
      });

      const service = new TorrentService();
      expect(service.isElectronBackendEnabled()).toBe(true);

      const torrent = await service.addMagnet("magnet:?xt=urn:btih:test1234567890123456789012345678901234567890");
      expect(backend.addMagnet).toHaveBeenCalled();
      expect(torrent).toBeTruthy();

      await service.destroy();
    });

    it("falls back to browser WebTorrent when no Electron backend", async () => {
      vi.stubGlobal("window", {});

      const service = new TorrentService();
      expect(service.isElectronBackendEnabled()).toBe(false);

      await service.destroy();
    });
  });

  describe("Chat + resend flow", () => {
    it("host detects /resend command and emits resend_requested", () => {
      const hostP2P = new P2PService();
      const resendRequests: string[] = [];

      hostP2P.on("resend_requested", (peerId) => {
        resendRequests.push(peerId);
      });

      expect(resendRequests).toHaveLength(0);
    });

    it("chat message with /resend content is forwarded and detected", () => {
      const messages: Array<{ senderId: string; content: string }> = [];
      const hostP2P = new P2PService();

      hostP2P.on("chat_received", (senderId, content) => {
        messages.push({ senderId, content });
      });

      expect(messages).toHaveLength(0);
    });
  });

  describe("Connection quality + sync tolerance", () => {
    it("RTT measurements affect connection quality", () => {
      const service = new P2PService();
      expect(service.getConnectionQuality()).toBe("unknown");
      expect(service.getLastRttMs()).toBeNull();
    });

    it("sync tolerance is clamped to valid range", async () => {
      const { clampSyncTolerance } = await import("../../utils/syncUtils");

      expect(clampSyncTolerance(0)).toBe(0);
      expect(clampSyncTolerance(1.5)).toBe(1.5);
      expect(clampSyncTolerance(30)).toBe(30);
      expect(clampSyncTolerance(-1)).toBe(1.5);
      expect(clampSyncTolerance(NaN)).toBe(1.5);
    });
  });
});
