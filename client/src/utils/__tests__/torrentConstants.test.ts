// @vitest-environment jsdom
import { describe, it, expect } from "vitest";
import { isValidMagnetLinkFormat, MAX_MAGNET_LINK_LENGTH } from "../torrentConstants";

describe("torrentConstants", () => {
  describe("MAX_MAGNET_LINK_LENGTH", () => {
    it("exports a positive number", () => {
      expect(typeof MAX_MAGNET_LINK_LENGTH).toBe("number");
      expect(MAX_MAGNET_LINK_LENGTH).toBeGreaterThan(0);
    });

    it("has a reasonable value for magnet links", () => {
      // Magnet links can be long, but typically under 10KB
      expect(MAX_MAGNET_LINK_LENGTH).toBeGreaterThanOrEqual(1000);
      expect(MAX_MAGNET_LINK_LENGTH).toBeLessThanOrEqual(100_000);
    });
  });

  describe("isValidMagnetLinkFormat", () => {
    describe("Valid magnet links", () => {
      it("validates standard btih magnet link", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates btih with uppercase hex", () => {
        const magnet = "magnet:?xt=urn:btih:ABC123DEF456ABC123DEF456ABC123DEF456ABC1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates btih with mixed case hex", () => {
        const magnet = "magnet:?xt=urn:btih:AbC123dEf456aBc123DeF456AbC123dEf456AbC1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates btmh magnet link", () => {
        const magnet = "magnet:?xt=urn:btmh:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates sha1 magnet link", () => {
        const magnet = "magnet:?xt=urn:sha1:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates ed2k magnet link with 32 hex chars", () => {
        // ed2k hash must be exactly 32 hex characters
        const magnet = "magnet:?xt=urn:ed2k:0123456789abcdef0123456789abcdef";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates magnet link with additional parameters", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1&dn=Test+File&tr=udp://tracker.example.com";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates magnet link with display name", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1&dn=My+Video+File.mp4";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("validates magnet link with tracker", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1&tr=udp://tracker.openbittorrent.com:80";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });
    });

    describe("Invalid magnet links", () => {
      it("rejects empty string", () => {
        expect(isValidMagnetLinkFormat("")).toBe(false);
      });

      it("rejects non-magnet URI", () => {
        expect(isValidMagnetLinkFormat("https://example.com/file.torrent")).toBe(false);
      });

      it("rejects magnet without query string", () => {
        expect(isValidMagnetLinkFormat("magnet:")).toBe(false);
      });

      it("rejects magnet without xt parameter", () => {
        expect(isValidMagnetLinkFormat("magnet:?dn=Test+File")).toBe(false);
      });

      it("rejects magnet with invalid xt format", () => {
        expect(isValidMagnetLinkFormat("magnet:?xt=invalid")).toBe(false);
      });

      it("rejects magnet with too short btih hash", () => {
        const magnet = "magnet:?xt=urn:btih:abc123";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with too long btih hash", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc123";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with invalid hex characters in btih", () => {
        const magnet = "magnet:?xt=urn:btih:xyz123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with too short ed2k hash", () => {
        const magnet = "magnet:?xt=urn:ed2k:abc123";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with too long ed2k hash", () => {
        const magnet = "magnet:?xt=urn:ed2k:abc123def456abc123def456abc123def456";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with invalid hex characters in ed2k", () => {
        const magnet = "magnet:?xt=urn:ed2k:xyz123def456abc123def456abcdef01";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with unknown hash type", () => {
        const magnet = "magnet:?xt=urn:unknown:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with spaces in hash", () => {
        const magnet = "magnet:?xt=urn:btih:abc 123 def 456 abc 123 def 456 abc 123 def 456 abc 1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet with special characters in hash", () => {
        const magnet = "magnet:?xt=urn:btih:abc!23def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });
    });

    describe("Edge cases", () => {
      it("handles magnet link with only xt parameter", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("handles magnet link with empty xt value", () => {
        const magnet = "magnet:?xt=";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("handles magnet link with case-insensitive prefix", () => {
        // The function should handle the standard magnet:? prefix
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("handles very long magnet link", () => {
        const dn = "dn=" + "a".repeat(1000);
        const magnet = `magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1&${dn}`;
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("handles magnet link with multiple trackers", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1&tr=udp://tracker1.com&tr=udp://tracker2.com";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("handles magnet link with exact 40 hex chars btih", () => {
        const magnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("handles magnet link with exact 32 hex chars ed2k", () => {
        const magnet = "magnet:?xt=urn:ed2k:0123456789abcdef0123456789abcdef";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("rejects magnet link with 39 hex chars btih", () => {
        const magnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef0123456";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("rejects magnet link with 31 hex chars ed2k", () => {
        const magnet = "magnet:?xt=urn:ed2k:0123456789abcdef0123456789abcde";
        expect(isValidMagnetLinkFormat(magnet)).toBe(false);
      });

      it("handles magnet link with 'urn:' prefix in xt", () => {
        const magnet = "magnet:?xt=urn:btih:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });

      it("handles magnet link without 'urn:' prefix in xt", () => {
        const magnet = "magnet:?xt=btih:abc123def456abc123def456abc123def456abc1";
        expect(isValidMagnetLinkFormat(magnet)).toBe(true);
      });
    });
  });
});
