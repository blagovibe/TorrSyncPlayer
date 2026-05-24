// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import VideoPlayer from "../VideoPlayer";
import type { AudioTrackInfo, SubtitleTrackInfo } from "../../services/types";

describe("VideoPlayer", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  const defaultProps = {
    videoRef: { current: null } as React.RefObject<HTMLVideoElement | null>,
    mediaLabel: null as string | null,
    mediaKind: null as "video" | "audio" | null,
    statusMessage: null as string | null,
    canControlPlayback: true,
    canControlSeek: false,
    canControlAudioTracks: false,
    canControlSubtitleTracks: false,
    fallbackAudioTracks: [] as AudioTrackInfo[],
    selectedAudioTrackIndex: null as number | null,
    fallbackSubtitles: [] as SubtitleTrackInfo[],
    selectedSubtitleIndex: null as number | null,
    onPlaybackStart: vi.fn(),
    onPlayerReady: vi.fn(),
    onBufferingChange: vi.fn(),
    onTimeUpdate: vi.fn(),
    onSeek: vi.fn(),
    onAudioTrackChange: vi.fn(),
    onSubtitleTrackChange: vi.fn(),
    onMuxStreamRequest: vi.fn(),
    isHost: false,
  };

  it("renders video element", () => {
    const { container } = render(<VideoPlayer {...defaultProps} />);
    expect(container.querySelector("video")).toBeTruthy();
  });

  it("renders media label when provided", () => {
    render(<VideoPlayer {...defaultProps} mediaLabel="Test Video.mp4" />);
    expect(screen.getByText("Test Video.mp4")).toBeTruthy();
  });

  it("renders status message when provided", () => {
    render(<VideoPlayer {...defaultProps} statusMessage="Buffering..." />);
    expect(screen.getByText("Buffering...")).toBeTruthy();
  });

  it("renders play button", () => {
    render(<VideoPlayer {...defaultProps} canControlPlayback={true} />);
    expect(screen.getByText("Play")).toBeTruthy();
  });

  it("renders time display", () => {
    const { container } = render(<VideoPlayer {...defaultProps} />);
    expect(container.querySelector(".time-label")).toBeTruthy();
    expect(container.querySelector(".time-label")?.textContent).toContain("0:00");
  });

  it("renders volume controls", () => {
    const { container } = render(<VideoPlayer {...defaultProps} />);
    expect(container.querySelector('input[type="range"][aria-label="Volume"]')).toBeTruthy();
  });

  it("renders fullscreen button", () => {
    render(<VideoPlayer {...defaultProps} />);
    expect(screen.getByText("Fullscreen")).toBeTruthy();
  });

  it("renders settings button", () => {
    render(<VideoPlayer {...defaultProps} />);
    expect(screen.getByText("Settings")).toBeTruthy();
  });
});
