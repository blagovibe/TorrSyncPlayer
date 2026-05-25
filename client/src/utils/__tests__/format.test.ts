import { describe, it, expect } from "vitest";
import { formatBytes, formatSpeed } from "../format";

describe("formatBytes", () => {
  it("returns 0 B for zero", () => {
    expect(formatBytes(0)).toBe("0 B");
  });

  it("returns 0 B for negative", () => {
    expect(formatBytes(-1)).toBe("0 B");
  });

  it("returns 0 B for NaN", () => {
    expect(formatBytes(NaN)).toBe("0 B");
  });

  it("returns 0 B for Infinity", () => {
    expect(formatBytes(Infinity)).toBe("0 B");
  });

  it("formats bytes", () => {
    expect(formatBytes(500)).toBe("500 B");
  });

  it("formats KiB", () => {
    expect(formatBytes(1024)).toBe("1 KiB");
  });

  it("formats MiB", () => {
    expect(formatBytes(1024 * 1024)).toBe("1 MiB");
  });

  it("formats GiB", () => {
    expect(formatBytes(1024 * 1024 * 1024)).toBe("1 GiB");
  });

  it("formats TiB", () => {
    expect(formatBytes(1024 * 1024 * 1024 * 1024)).toBe("1 TiB");
  });

  it("formats fractional KiB", () => {
    expect(formatBytes(1536)).toBe("1.5 KiB");
  });

  it("formats fractional MiB", () => {
    expect(formatBytes(1024 * 1024 * 2.5)).toBe("2.5 MiB");
  });

  it("uses 0 decimal for values >= 100", () => {
    expect(formatBytes(100 * 1024)).toBe("100 KiB");
  });

  it("uses 1 decimal for values >= 10", () => {
    expect(formatBytes(15 * 1024 * 1024)).toBe("15 MiB");
  });
});

describe("formatSpeed", () => {
  it("returns 0 B/s for zero", () => {
    expect(formatSpeed(0)).toBe("0 B/s");
  });

  it("returns 0 B/s for negative", () => {
    expect(formatSpeed(-1)).toBe("0 B/s");
  });

  it("formats B/s", () => {
    expect(formatSpeed(100)).toBe("100 B/s");
  });

  it("formats KiB/s", () => {
    expect(formatSpeed(1024)).toBe("1 KiB/s");
  });

  it("formats MiB/s", () => {
    expect(formatSpeed(1024 * 1024)).toBe("1 MiB/s");
  });

  it("formats GiB/s", () => {
    expect(formatSpeed(1024 * 1024 * 1024)).toBe("1 GiB/s");
  });
});
