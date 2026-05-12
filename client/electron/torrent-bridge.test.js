import { EventEmitter } from "node:events";
import { afterEach, describe, expect, it, vi } from "vitest";

const torrentBridgeModule = await import("./torrent-bridge.cjs");
const { TorrentBridge, formatTorrentSnapshot } = torrentBridgeModule.default ?? torrentBridgeModule;

function createFakeServer(port = 4321) {
  const underlyingServer = new EventEmitter();

  const server = {
    pathname: "/webtorrent",
    server: underlyingServer,
    listen: vi.fn((_port, _host, callback) => {
      callback?.();
      return server;
    }),
    address: vi.fn(() => ({ address: "127.0.0.1", port })),
    close: vi.fn((callback) => {
      callback?.();
    }),
  };

  return server;
}

function createFakeTorrent(client, { infoHash = "hash-1", fileName = "movie.mkv" } = {}) {
  const torrent = {
    infoHash,
    progress: 0.42,
    downloadSpeed: 2048,
    numPeers: 3,
    files: [],
    on: vi.fn(),
    destroy: vi.fn((callback) => {
      callback?.();
    }),
  };

  torrent.files = [
    {
      name: fileName,
      path: fileName,
      length: 1_048_576,
      progress: 0.75,
      get streamURL() {
        if (!client._server) {
          throw new Error("No server created");
        }

        return `${client._server.pathname}/${torrent.infoHash}/${this.path}`;
      },
    },
  ];

  return torrent;
}

function createFakeClient() {
  const client = {
    _server: null,
    destroyed: false,
    createServer: vi.fn(() => {
      const server = createFakeServer();
      client._server = server;
      return server;
    }),
    add: vi.fn((torrentSource, onTorrent) => {
      const torrent = createFakeTorrent(client, {
        infoHash: "hash-1",
        fileName: "movie.mkv",
      });

      queueMicrotask(() => onTorrent(torrent));
      return torrent;
    }),
    destroy: vi.fn((callback) => {
      client.destroyed = true;
      callback?.();
    }),
  };

  return client;
}

function linkBridgeToClient(bridge, client) {
  bridge.client = client;
  bridge.clientPromise = Promise.resolve(client);
  bridge.getClient = vi.fn().mockResolvedValue(client);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("torrent bridge", () => {
  it("creates a WebTorrent server before reading stream URLs", async () => {
    const bridge = new TorrentBridge();
    const client = createFakeClient();
    linkBridgeToClient(bridge, client);

    const snapshot = await bridge.addTorrentFile(new Uint8Array([1, 2, 3]));

    expect(client.createServer).toHaveBeenCalledWith({ origin: "*" });
    expect(client.add).toHaveBeenCalledWith(expect.any(Uint8Array), expect.any(Function));
    expect(snapshot.files[0]).toEqual(
      expect.objectContaining({
        name: "movie.mkv",
        streamUrl: "http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv",
      }),
    );
  });

  it("parses audio track metadata from ffprobe output", async () => {
    const bridge = new TorrentBridge();
    const runFfprobe = vi.spyOn(bridge, "runFfprobe").mockResolvedValue({
      stdout: JSON.stringify({
        streams: [
          {
            codec_name: "aac",
            codec_long_name: "AAC (Advanced Audio Coding)",
            channels: 2,
            sample_rate: "48000",
            tags: {
              language: "eng",
              title: "English Stereo",
            },
          },
          {
            codec_name: "opus",
            codec_long_name: "Opus",
            channels: 6,
            sample_rate: "48000",
            tags: {
              language: "jpn",
            },
          },
        ],
      }),
      stderr: "",
    });

    const tracks = await bridge.probeAudioTracks("http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv");

    expect(runFfprobe).toHaveBeenCalledWith("http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv");
    expect(tracks).toEqual([
      {
        index: 0,
        label: "English Stereo",
        language: "eng",
        codecName: "aac",
        channels: 2,
        sampleRate: 48000,
      },
      {
        index: 1,
        label: "Opus",
        language: "jpn",
        codecName: "opus",
        channels: 6,
        sampleRate: 48000,
      },
    ]);
  });

  it("creates a reusable temporary stream URL for fallback audio", async () => {
    const bridge = new TorrentBridge();
    const ensureAudioServer = vi.spyOn(bridge, "ensureAudioServer").mockResolvedValue("http://127.0.0.1:9999");

    const url = await bridge.createAudioTrackStreamUrl({
      streamUrl: "http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv",
      trackIndex: 1,
      startSeconds: 12.5,
    });

    expect(ensureAudioServer).toHaveBeenCalledOnce();
    expect(url).toMatch(/^http:\/\/127\.0\.0\.1:9999\/audio\/[a-z0-9-]+$/);
    expect(bridge.audioSessions.size).toBe(1);

    const [session] = [...bridge.audioSessions.values()];
    expect(session).toEqual(
      expect.objectContaining({
        streamUrl: "http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv",
        trackIndex: 1,
        startSeconds: 12.5,
        process: null,
      }),
    );
  });

  it("reuses the same server for subsequent torrent loads", async () => {
    const bridge = new TorrentBridge();
    const client = createFakeClient();
    linkBridgeToClient(bridge, client);

    const firstSnapshot = await bridge.addTorrentFile(new Uint8Array([1, 2, 3]));
    const secondSnapshot = await bridge.addTorrentFile(new Uint8Array([4, 5, 6]));

    expect(client.createServer).toHaveBeenCalledTimes(1);
    expect(firstSnapshot.files[0].streamUrl).toBe("http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv");
    expect(secondSnapshot.files[0].streamUrl).toBe("http://127.0.0.1:4321/webtorrent/hash-1/movie.mkv");
  });

  it("formats torrent snapshots with an absolute stream origin", () => {
    const torrent = {
      progress: 0.11,
      downloadSpeed: 100,
      numPeers: 1,
      files: [
        {
          name: "clip.webm",
          path: "clip.webm",
          length: 2_000,
          progress: 0.25,
          streamURL: "/webtorrent/hash-xyz/clip.webm",
        },
      ],
    };

    const snapshot = formatTorrentSnapshot(torrent, 2, "http://127.0.0.1:4321");

    expect(snapshot).toEqual(
      expect.objectContaining({
        progress: 0.11,
        downloadSpeed: 100,
        numPeers: 1,
        discoveredPeerCount: 2,
      }),
    );
    expect(snapshot.files[0]).toEqual(
      expect.objectContaining({
        name: "clip.webm",
        streamUrl: "http://127.0.0.1:4321/webtorrent/hash-xyz/clip.webm",
      }),
    );
  });

  it("destroys the cached client on shutdown", async () => {
    const bridge = new TorrentBridge();
    const client = createFakeClient();
    linkBridgeToClient(bridge, client);

    await bridge.addTorrentFile(new Uint8Array([1, 2, 3]));
    await bridge.destroy();

    expect(client.destroy).toHaveBeenCalledOnce();
    expect(bridge.client).toBeNull();
    expect(bridge.clientPromise).toBeNull();
    expect(bridge.streamBaseUrl).toBeNull();
  });
});
