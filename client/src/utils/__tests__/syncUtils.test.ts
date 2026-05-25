import { describe, it, expect } from "vitest";
import { clampSyncTolerance, isPlaybackBlockedError } from "../syncUtils";

describe("clampSyncTolerance", () => {
  it("returns default for NaN", () => {
    expect(clampSyncTolerance(NaN)).toBe(1.5);
  });

  it("returns default for negative values", () => {
    expect(clampSyncTolerance(-1)).toBe(1.5);
  });

  it("returns default for -Infinity", () => {
    expect(clampSyncTolerance(-Infinity)).toBe(1.5);
  });

  it("returns default for Infinity", () => {
    expect(clampSyncTolerance(Infinity)).toBe(1.5);
  });

  it("returns 0 for zero", () => {
    expect(clampSyncTolerance(0)).toBe(0);
  });

  it("returns the value for valid positive numbers", () => {
    expect(clampSyncTolerance(2.5)).toBe(2.5);
  });

  it("returns the value for 1", () => {
    expect(clampSyncTolerance(1)).toBe(1);
  });
});

describe("isPlaybackBlockedError", () => {
  it("returns false for null", () => {
    expect(isPlaybackBlockedError(null)).toBe(false);
  });

  it("returns false for undefined", () => {
    expect(isPlaybackBlockedError(undefined)).toBe(false);
  });

  it("returns false for a string", () => {
    expect(isPlaybackBlockedError("some error")).toBe(false);
  });

  it("returns false for a generic Error", () => {
    expect(isPlaybackBlockedError(new Error("generic error"))).toBe(false);
  });

  it("returns true for NotAllowedError by name", () => {
    const error = new Error("The operation is not allowed.");
    error.name = "NotAllowedError";
    expect(isPlaybackBlockedError(error)).toBe(true);
  });

  it("returns true for autoplay-blocked message", () => {
    const error = new DOMException("play() failed because the user didn't interact with the document first.", "NotAllowedError");
    expect(isPlaybackBlockedError(error)).toBe(true);
  });

  it("returns false for unrelated DOMException", () => {
    const error = new DOMException("Security error", "SecurityError");
    expect(isPlaybackBlockedError(error)).toBe(false);
  });
});
