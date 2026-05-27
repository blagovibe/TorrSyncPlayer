// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import VideoPlayer from "../VideoPlayer";
import HomePage from "../HomePage";
import type { AudioTrackInfo, SubtitleTrackInfo } from "../../services/types";

describe("Error states", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  describe("VideoPlayer error states", () => {
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
    };

    it("renders error status message", () => {
      render(<VideoPlayer {...defaultProps} statusMessage="Failed to load media" />);
      expect(screen.getByText("Failed to load media")).toBeTruthy();
    });

    it("renders autoplay blocked message", () => {
      render(<VideoPlayer {...defaultProps} statusMessage="Autoplay was blocked. Press Play in the player to start the movie." />);
      expect(screen.getByText(/Autoplay was blocked/)).toBeTruthy();
    });

    it("renders buffering indicator", () => {
      render(<VideoPlayer {...defaultProps} statusMessage="Buffering..." />);
      expect(screen.getByText("Buffering...")).toBeTruthy();
    });

    it("disables controls when canControlPlayback is false", () => {
      render(<VideoPlayer {...defaultProps} canControlPlayback={false} />);
      const playButton = screen.getByText("Play");
      expect(playButton).toHaveProperty("disabled", true);
    });

    it("disables seek when canControlSeek is false", () => {
      render(<VideoPlayer {...defaultProps} canControlSeek={false} />);
      const seekInput = screen.getByLabelText("Seek");
      expect(seekInput).toHaveProperty("disabled", true);
    });

    it("renders placeholder when no media is loaded", () => {
      render(<VideoPlayer {...defaultProps} />);
      expect(screen.getByText("Load a torrent to start playback")).toBeTruthy();
    });

    it("renders audio mode class when mediaKind is audio", () => {
      const { container } = render(<VideoPlayer {...defaultProps} mediaKind="audio" />);
      expect(container.querySelector(".audio-mode")).toBeTruthy();
    });
  });

  describe("HomePage error states", () => {
    it("renders connection error message", () => {
      render(
        <HomePage
          peerId=""
          onCreateRoom={vi.fn()}
          onJoinRoom={vi.fn()}
          isConnecting={false}
          connectionError="Connection timed out. The host may be offline."
        />
      );
      expect(screen.getByText(/Connection timed out/)).toBeTruthy();
    });

    it("renders connecting state", () => {
      render(
        <HomePage
          peerId=""
          onCreateRoom={vi.fn()}
          onJoinRoom={vi.fn()}
          isConnecting={true}
          connectionError={null}
        />
      );
      expect(screen.getByText(/Creating\u2026/)).toBeTruthy();
    });

    it("disables buttons while connecting", () => {
      render(
        <HomePage
          peerId=""
          onCreateRoom={vi.fn()}
          onJoinRoom={vi.fn()}
          isConnecting={true}
          connectionError={null}
        />
      );
      const buttons = screen.getAllByRole("button");
      const disabledButtons = buttons.filter(btn => (btn as HTMLButtonElement).disabled);
      expect(disabledButtons.length).toBeGreaterThan(0);
    });

    it("renders peer ID when available", () => {
      render(
        <HomePage
          peerId="ABC123"
          onCreateRoom={vi.fn()}
          onJoinRoom={vi.fn()}
          isConnecting={false}
          connectionError={null}
        />
      );
      expect(screen.getByText("ABC123")).toBeTruthy();
    });
  });
});
