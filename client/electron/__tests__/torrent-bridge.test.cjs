const { TorrentBridge, formatTorrentSnapshot } = require("../torrent-bridge.cjs");

function createMockTorrent(files = []) {
  return {
    files: files.map((f, i) => ({
      name: f.name || `file-${i}.mp4`,
      length: f.length || 1000,
      progress: f.progress || 0,
      streamURL: f.streamUrl || `http://127.0.0.1:4321/webtorrent/hash/${f.name || `file-${i}.mp4`}`,
    })),
    progress: 0.5,
    downloadSpeed: 1024,
    numPeers: 3,
    discoveredPeerCount: 5,
  };
}

describe("formatTorrentSnapshot", () => {
  it("formats a basic torrent snapshot", () => {
    const torrent = createMockTorrent([{ name: "movie.mp4", length: 5000 }]);
    const result = formatTorrentSnapshot(torrent, 10, "http://127.0.0.1:4321");

    expect(result.progress).toBe(0.5);
    expect(result.downloadSpeed).toBe(1024);
    expect(result.numPeers).toBe(3);
    expect(result.discoveredPeerCount).toBe(10);
    expect(result.files).toHaveLength(1);
    expect(result.files[0].name).toBe("movie.mp4");
    expect(result.files[0].kind).toBe("video");
  });

  it("sorts video files before audio files", () => {
    const torrent = createMockTorrent([
      { name: "song.mp3", length: 5000 },
      { name: "movie.mp4", length: 10000 },
    ]);
    const result = formatTorrentSnapshot(torrent, 0);

    expect(result.files).toHaveLength(2);
    expect(result.files[0].kind).toBe("video");
    expect(result.files[1].kind).toBe("audio");
  });

  it("sorts by size within same kind", () => {
    const torrent = createMockTorrent([
      { name: "small.mp4", length: 1000 },
      { name: "large.mp4", length: 10000 },
    ]);
    const result = formatTorrentSnapshot(torrent, 0);

    expect(result.files[0].name).toBe("large.mp4");
    expect(result.files[1].name).toBe("small.mp4");
  });

  it("filters out non-media files", () => {
    const torrent = createMockTorrent([
      { name: "movie.mp4", length: 5000 },
      { name: "readme.txt", length: 100 },
      { name: "song.mp3", length: 3000 },
    ]);
    const result = formatTorrentSnapshot(torrent, 0);

    expect(result.files).toHaveLength(2);
    expect(result.files.map(f => f.name)).not.toContain("readme.txt");
  });

  it("includes stream URL from base URL", () => {
    const torrent = createMockTorrent([{ name: "movie.mkv", length: 5000 }]);
    const result = formatTorrentSnapshot(torrent, 0, "http://127.0.0.1:4321");

    expect(result.files[0].streamUrl).toContain("http://127.0.0.1:4321");
  });

  it("throws on too many files", () => {
    const files = Array.from({ length: 10001 }, (_, i) => ({ name: `file-${i}.mp4`, length: 100 }));
    const torrent = createMockTorrent(files);

    expect(() => formatTorrentSnapshot(torrent, 0)).toThrow(/too many files/i);
  });

  it("throws on file name exceeding max length", () => {
    const longName = "a".repeat(600) + ".mp4";
    const torrent = createMockTorrent([{ name: longName, length: 1000 }]);

    expect(() => formatTorrentSnapshot(torrent, 0)).toThrow(/file name exceeds maximum length/i);
  });

  it("handles torrent with no files", () => {
    const torrent = createMockTorrent([]);
    const result = formatTorrentSnapshot(torrent, 0);

    expect(result.files).toHaveLength(0);
  });

  it("handles missing optional fields", () => {
    const torrent = {
      files: [{ name: "movie.mp4" }],
    };
    const result = formatTorrentSnapshot(torrent, 0);

    expect(result.progress).toBe(0);
    expect(result.downloadSpeed).toBe(0);
    expect(result.numPeers).toBe(0);
    expect(result.files[0].length).toBe(0);
    expect(result.files[0].progress).toBe(0);
  });
});

describe("TorrentBridge", () => {
  let bridge;

  beforeEach(() => {
    bridge = new TorrentBridge();
  });

  afterEach(async () => {
    try {
      await bridge.destroy();
    } catch {
      // Ignore cleanup errors
    }
  });

  it("initializes with default buffer size", () => {
    expect(bridge.maxBufferBytes).toBe(500 * 1024 * 1024);
  });

  it("setMaxBufferMB updates buffer size", () => {
    bridge.setMaxBufferMB(200);
    expect(bridge.maxBufferBytes).toBe(200 * 1024 * 1024);
  });

  it("setMaxBufferMB enforces minimum of 1 MB", () => {
    bridge.setMaxBufferMB(0);
    expect(bridge.maxBufferBytes).toBe(1 * 1024 * 1024);

    bridge.setMaxBufferMB(-10);
    expect(bridge.maxBufferBytes).toBe(1 * 1024 * 1024);
  });

  it("getStats returns null when no active torrent", async () => {
    const stats = await bridge.getStats();
    expect(stats).toBeNull();
  });

  it("clear is safe when no active torrent", async () => {
    await expect(bridge.clear()).resolves.toBeUndefined();
  });

  it("destroy is safe when no client", async () => {
    await expect(bridge.destroy()).resolves.toBeUndefined();
  });

  it("addMagnet rejects invalid magnet link format", async () => {
    await expect(bridge.addMagnet("not-a-magnet-link")).rejects.toThrow(/Invalid magnet link format/);
  });

  it("addMagnet rejects oversized magnet link", async () => {
    const longMagnet = "magnet:?xt=urn:btih:" + "a".repeat(40) + "&" + "x".repeat(9000);
    await expect(bridge.addMagnet(longMagnet)).rejects.toThrow(/too long/);
  });

  it("addTorrentFile rejects non-Uint8Array input", async () => {
    await expect(bridge.addTorrentFile("not-bytes")).rejects.toThrow(/Invalid torrent source type/);
  });

  it("addTorrentFile accepts Array input", async () => {
    const arrayInput = [100, 58, 97, 110, 110, 111, 117, 110, 99, 101];
    let errorThrown = false;
    try {
      await bridge.addTorrentFile(arrayInput);
    } catch (error) {
      errorThrown = true;
      expect(error.message).not.toMatch(/Invalid torrent source type/);
    }
  });

  it("validateLocalStreamUrl rejects non-http protocols", () => {
    expect(() => bridge.validateLocalStreamUrl("ftp://example.com/file.mp4")).toThrow(/must use http or https/);
  });

  it("validateLocalStreamUrl rejects non-loopback hosts", () => {
    expect(() => bridge.validateLocalStreamUrl("http://192.168.1.1:8080/file.mp4")).toThrow(/must point to a local address/);
  });

  it("validateLocalStreamUrl accepts 127.0.0.1", () => {
    expect(() => bridge.validateLocalStreamUrl("http://127.0.0.1:8080/file.mp4")).not.toThrow();
  });

  it("validateLocalStreamUrl rejects non-loopback IPv6", () => {
    expect(() => bridge.validateLocalStreamUrl("http://[2001:db8::1]:8080/file.mp4")).toThrow(/must point to a local address/);
  });

  it("validateLocalStreamUrl rejects invalid URLs", () => {
    expect(() => bridge.validateLocalStreamUrl("not-a-url")).toThrow(/Invalid stream URL/);
  });

  it("clearAudioSessions cleans up all sessions", () => {
    bridge.audioSessions.set("test-token", {
      cleanupTimer: setTimeout(() => {}, 10000),
      process: { killed: false, kill: vi.fn() },
    });

    bridge.clearAudioSessions();
    expect(bridge.audioSessions.size).toBe(0);
  });

  it("setStreamBaseUrl resolves pending waiter", () => {
    bridge._ensureAudioServerWaiter = null;
    bridge._resolveAudioServerWaiter = null;

    bridge.setStreamBaseUrl("http://127.0.0.1:9999");
    expect(bridge.audioServerBaseUrl).toBe("http://127.0.0.1:9999");
  });
});
