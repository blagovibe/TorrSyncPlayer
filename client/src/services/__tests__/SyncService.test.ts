import { afterEach, describe, expect, it, vi } from "vitest";
import { SyncService } from "../SyncService";
import type SignalingService from "../SignalingService";

type VideoEventName = "play" | "pause" | "seeked";

function createVideo(initialTime = 0): HTMLVideoElement & { dispatch: (event: VideoEventName) => void } {
  const listeners = new Map<VideoEventName, Set<() => void>>();
  const video = {
    currentTime: initialTime,
    play: vi.fn().mockResolvedValue(undefined),
    pause: vi.fn(),
    addEventListener: vi.fn((event: VideoEventName, callback: () => void) => {
      const callbacks = listeners.get(event) ?? new Set<() => void>();
      callbacks.add(callback);
      listeners.set(event, callbacks);
    }),
    removeEventListener: vi.fn((event: VideoEventName, callback: () => void) => {
      listeners.get(event)?.delete(callback);
    }),
    dispatch: (event: VideoEventName) => {
      for (const callback of listeners.get(event) ?? []) {
        callback();
      }
    },
  };

  return video as unknown as HTMLVideoElement & { dispatch: (event: VideoEventName) => void };
}

function createSignaling(): SignalingService {
  return {
    sendSync: vi.fn(),
  } as unknown as SignalingService;
}

describe("SyncService", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends sync commands for master controls", () => {
    const signaling = createSignaling();
    const video = createVideo(8);
    const service = new SyncService(signaling, video, "master");
    const outbound = vi.fn();
    service.on("outbound_sync", outbound);

    service.play();
    service.pause();
    service.seek(42);

    expect(video.play).toHaveBeenCalledOnce();
    expect(video.pause).toHaveBeenCalledOnce();
    expect(video.currentTime).toBe(42);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(1, "play", 8);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(2, "pause", 8);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(3, "seek", 42);
    expect(outbound).toHaveBeenNthCalledWith(1, { action: "play", position: 8, server_ts: expect.any(Number) });
    expect(outbound).toHaveBeenNthCalledWith(2, { action: "pause", position: 8, server_ts: expect.any(Number) });
    expect(outbound).toHaveBeenNthCalledWith(3, {
      action: "seek",
      position: 42,
      server_ts: expect.any(Number),
    });
  });

  it("binds master video events to outbound sync messages", () => {
    const signaling = createSignaling();
    const video = createVideo(15);
    const service = new SyncService(signaling, video, "master");
    const outbound = vi.fn();
    service.on("outbound_sync", outbound);

    video.dispatch("play");
    video.dispatch("pause");
    video.dispatch("seeked");

    expect(signaling.sendSync).toHaveBeenNthCalledWith(1, "play", 15);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(2, "pause", 15);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(3, "seek", 15);
    expect(outbound).toHaveBeenCalledTimes(3);
  });

  it("suppresses immediate duplicate master events after explicit controls", () => {
    const signaling = createSignaling();
    const video = createVideo(20);
    const service = new SyncService(signaling, video, "master");
    const outbound = vi.fn();
    service.on("outbound_sync", outbound);

    service.play();
    video.dispatch("play");
    service.pause();
    video.dispatch("pause");
    service.seek(33);
    video.dispatch("seeked");

    expect(signaling.sendSync).toHaveBeenCalledTimes(3);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(1, "play", 20);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(2, "pause", 20);
    expect(signaling.sendSync).toHaveBeenNthCalledWith(3, "seek", 33);
    expect(outbound).toHaveBeenCalledTimes(3);
  });

  it("removes master video listeners on dispose", () => {
    const signaling = createSignaling();
    const video = createVideo(15);
    const service = new SyncService(signaling, video, "master");

    service.dispose();
    video.dispatch("play");

    expect(video.removeEventListener).toHaveBeenCalledTimes(3);
    expect(signaling.sendSync).not.toHaveBeenCalled();
  });

  it("applies remote slave sync with latency compensation and emits events", () => {
    vi.spyOn(Date, "now").mockReturnValue(10_000);
    const video = createVideo();
    const service = new SyncService(createSignaling(), video, "slave");
    const syncPlay = vi.fn();
    const syncPause = vi.fn();
    const syncSeek = vi.fn();

    service.on("sync_play", syncPlay);
    service.on("sync_pause", syncPause);
    service.on("sync_seek", syncSeek);

    const playMessage = { action: "play" as const, position: 3, server_ts: 8_000 };
    const pauseMessage = { action: "pause" as const, position: 7, server_ts: 9_000 };
    const seekMessage = { action: "seek" as const, position: 11, server_ts: 10_000 };
    service.applyRemoteSync(playMessage);
    service.applyRemoteSync(pauseMessage);
    service.applyRemoteSync(seekMessage);

    expect(video.play).toHaveBeenCalledOnce();
    expect(video.pause).toHaveBeenCalledOnce();
    expect(syncPlay).toHaveBeenCalledWith(playMessage);
    expect(syncPause).toHaveBeenCalledWith(pauseMessage);
    expect(syncSeek).toHaveBeenCalledWith(seekMessage);
    expect(video.currentTime).toBe(11);
  });

  it("ignores remote sync while in master mode", () => {
    const video = createVideo(2);
    const service = new SyncService(createSignaling(), video, "master");

    service.applyRemoteSync({ action: "play", position: 9, server_ts: Date.now() });

    expect(video.currentTime).toBe(2);
    expect(video.play).not.toHaveBeenCalled();
  });
});
