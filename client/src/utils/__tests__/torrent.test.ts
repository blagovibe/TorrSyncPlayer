import { describe, it, expect } from "vitest";
import { isValidMagnetLink, hashBytes, createMagnetSource, createTorrentFileSource } from "../torrent";

describe("isValidMagnetLink", () => {
  it("validates a basic magnet link with btih", () => {
    expect(isValidMagnetLink("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567")).toBe(true);
  });

  it("validates uppercase btih hash", () => {
    expect(isValidMagnetLink("magnet:?xt=urn:btih:0123456789ABCDEF0123456789ABCDEF01234567")).toBe(true);
  });

  it("rejects empty string", () => {
    expect(isValidMagnetLink("")).toBe(false);
  });

  it("rejects non-magnet URL", () => {
    expect(isValidMagnetLink("https://example.com/file.torrent")).toBe(false);
  });

  it("rejects magnet without xt parameter", () => {
    expect(isValidMagnetLink("magnet:?dn=filename")).toBe(false);
  });

  it("rejects invalid hash length", () => {
    expect(isValidMagnetLink("magnet:?xt=urn:btih:abc123")).toBe(false);
  });

  it("validates magnet link with wss tracker", () => {
    const link = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=wss%3A%2F%2Ftracker.btorrent.xyz";
    expect(isValidMagnetLink(link)).toBe(true);
  });

  it("rejects magnet link with http tracker", () => {
    const link = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=http%3A%2F%2Ftracker.example.com";
    expect(isValidMagnetLink(link)).toBe(false);
  });

  it("rejects magnet link with localhost tracker", () => {
    const link = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=http%3A%2F%2F127.0.0.1%3A8080";
    expect(isValidMagnetLink(link)).toBe(false);
  });

  it("rejects magnet link with 192.168 tracker", () => {
    const link = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=https%3A%2F%2F192.168.1.1";
    expect(isValidMagnetLink(link)).toBe(false);
  });

  it("validates magnet with btmh hash", () => {
    expect(isValidMagnetLink("magnet:?xt=urn:btmh:0123456789abcdef0123456789abcdef01234567")).toBe(true);
  });

  it("rejects magnet link exceeding max length", () => {
    const longLink = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=" + "x".repeat(8000);
    expect(isValidMagnetLink(longLink)).toBe(false);
  });
});

describe("hashBytes", () => {
  it("returns consistent hash for same input", () => {
    const bytes = new Uint8Array([1, 2, 3, 4, 5]);
    expect(hashBytes(bytes)).toBe(hashBytes(new Uint8Array([1, 2, 3, 4, 5])));
  });

  it("returns different hash for different input", () => {
    const a = new Uint8Array([1, 2, 3]);
    const b = new Uint8Array([4, 5, 6]);
    expect(hashBytes(a)).not.toBe(hashBytes(b));
  });

  it("returns 8-character hex string", () => {
    const bytes = new Uint8Array([1, 2, 3]);
    const hash = hashBytes(bytes);
    expect(hash).toMatch(/^[0-9a-f]{8}$/);
  });

  it("handles empty array", () => {
    const hash = hashBytes(new Uint8Array([]));
    expect(hash).toMatch(/^[0-9a-f]{8}$/);
  });
});

describe("createMagnetSource", () => {
  it("creates a valid magnet source", () => {
    const source = createMagnetSource("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567");
    expect(source.kind).toBe("magnet");
    if (source.kind !== "magnet") throw new Error("Expected magnet source");
    expect(source.magnetLink).toBe("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567");
    expect(source.sourceKey).toContain("magnet:");
  });

  it("trims whitespace from magnet link", () => {
    const source = createMagnetSource("  magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567  ");
    if (source.kind !== "magnet") throw new Error("Expected magnet source");
    expect(source.magnetLink).toBe("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567");
  });

  it("throws for invalid magnet link", () => {
    expect(() => createMagnetSource("not-a-magnet-link")).toThrow("Invalid magnet link format");
  });
});

describe("createTorrentFileSource", () => {
  it("creates a valid file source", () => {
    const bytes = new Uint8Array([1, 2, 3, 4]);
    const source = createTorrentFileSource("test.torrent", bytes);
    expect(source.kind).toBe("file");
    if (source.kind !== "file") throw new Error("Expected file source");
    expect(source.fileName).toBe("test.torrent");
    expect(source.bytes).toEqual([1, 2, 3, 4]);
    expect(source.sourceKey).toContain("file:test.torrent");
  });

  it("defaults filename to shared.torrent if empty", () => {
    const source = createTorrentFileSource("", new Uint8Array([1]));
    if (source.kind !== "file") throw new Error("Expected file source");
    expect(source.fileName).toBe("shared.torrent");
  });

  it("trims whitespace from filename", () => {
    const source = createTorrentFileSource("  test.torrent  ", new Uint8Array([1]));
    if (source.kind !== "file") throw new Error("Expected file source");
    expect(source.fileName).toBe("test.torrent");
  });
});
