import { describe, it, expect, vi } from "vitest";
import { createCleanup } from "../cleanup";

describe("createCleanup", () => {
  it("is not aborted initially", () => {
    const cleanup = createCleanup();
    expect(cleanup.aborted).toBe(false);
  });

  it("can add a cleanup function", () => {
    const cleanup = createCleanup();
    const fn = vi.fn();
    cleanup.add(fn);
    cleanup.abort();
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("cleans up on abort", () => {
    const cleanup = createCleanup();
    const fn1 = vi.fn();
    const fn2 = vi.fn();
    cleanup.add(fn1);
    cleanup.add(fn2);
    cleanup.abort();
    expect(fn1).toHaveBeenCalledTimes(1);
    expect(fn2).toHaveBeenCalledTimes(1);
    expect(cleanup.aborted).toBe(true);
  });

  it("is idempotent on double abort", () => {
    const cleanup = createCleanup();
    const fn = vi.fn();
    cleanup.add(fn);
    cleanup.abort();
    cleanup.abort();
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("signal is aborted after abort", () => {
    const cleanup = createCleanup();
    expect(cleanup.signal.aborted).toBe(false);
    cleanup.abort();
    expect(cleanup.signal.aborted).toBe(true);
  });

  it("manages setTimeout that fires", async () => {
    const cleanup = createCleanup();
    const fn = vi.fn();
    cleanup.setTimeout(fn, 10);
    await new Promise(r => setTimeout(r, 50));
    expect(fn).toHaveBeenCalledTimes(1);
  });

  it("cancels setTimeout on abort before it fires", async () => {
    const cleanup = createCleanup();
    const fn = vi.fn();
    cleanup.setTimeout(fn, 100);
    cleanup.abort();
    await new Promise(r => setTimeout(r, 200));
    expect(fn).not.toHaveBeenCalled();
  });

  it("clears a specific timer", async () => {
    const cleanup = createCleanup();
    const fn = vi.fn();
    const id = cleanup.setTimeout(fn, 100);
    cleanup.clearTimer(id);
    await new Promise(r => setTimeout(r, 200));
    expect(fn).not.toHaveBeenCalled();
  });

  it("can add multiple cleanup functions", () => {
    const cleanup = createCleanup();
    const fns = [vi.fn(), vi.fn(), vi.fn(), vi.fn(), vi.fn()];
    for (const fn of fns) cleanup.add(fn);
    cleanup.abort();
    for (const fn of fns) {
      expect(fn).toHaveBeenCalledTimes(1);
    }
  });
});
