import { describe, it, expect, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RoomPage from "../RoomPage";

const defaultProps = {
  connection: {
    peerId: "ABC123",
    peerRole: "master" as const,
    peers: [{ id: "self", name: "You", role: "master" as const, connectionState: "connected" as const }],
    isConnected: true,
    connectionQuality: "good" as const,
    rttMs: 50,
  },
  torrent: {
    magnetLink: "",
    torrentFileName: null,
    sharedSourceLabel: null,
    mediaFiles: [],
    selectedMediaIndex: null,
    selectedMediaLabel: null,
    selectedMediaKind: null,
    selectedMediaAudioTracks: [],
    selectedAudioTrackIndex: null,
    selectedSubtitles: [],
    selectedSubtitleIndex: null,
    isLoadingTorrent: false,
    downloadSpeed: "0 B/s",
    bufferingProgress: 0,
    torrentPeerCount: 0,
    torrentError: null,
    torrentPeerHint: "Looking for peers",
    bufferHint: "Load a torrent",
  },
  player: {
    videoRef: { current: null },
    playbackNotice: null,
    syncToleranceSeconds: 1.5,
    canControl: true,
  },
  chat: {
    chatMessages: [],
    onSendChat: vi.fn(),
  },
  ffmpegAvailable: null,
  onMagnetLinkChange: vi.fn(),
  onTorrentFileChange: vi.fn(),
  onPlaybackStarted: vi.fn(),
  onPlayerReady: vi.fn(),
  onAudioTrackChange: vi.fn(),
  onSubtitleTrackChange: vi.fn(),
  onLoadMagnet: vi.fn(),
  onLoadTorrentFile: vi.fn(),
  onSelectMediaFile: vi.fn(),
  onLeaveRoom: vi.fn(),
  onTimeUpdate: vi.fn(),
  onSyncToleranceChange: vi.fn(),
};

describe("RoomPage", () => {
  it("renders the room page with peer ID", () => {
    render(<RoomPage {...defaultProps} />);
    expect(screen.getByText(/ABC123/)).toBeInTheDocument();
    cleanup();
  });

  it("shows TorrentControlsPanel for master role", () => {
    render(<RoomPage {...defaultProps} />);
    expect(screen.getByText("Host controls")).toBeInTheDocument();
    cleanup();
  });

  it("shows GuestViewPanel for guest role", () => {
    const guestProps = {
      ...defaultProps,
      connection: { ...defaultProps.connection, peerRole: "slave" as const },
      player: { ...defaultProps.player, canControl: false },
    };
    render(<RoomPage {...guestProps} />);
    expect(screen.getByText("Guest view")).toBeInTheDocument();
    cleanup();
  });

  it("calls onTorrentFileChange on torrent file drop for master", async () => {
    const user = userEvent.setup();
    render(<RoomPage {...defaultProps} />);
    const section = document.querySelector(".room-page")!;
    await user.upload(section.querySelector(".file-picker input")!, new File(["test"], "test.torrent", { type: "application/x-bittorrent" }));
    cleanup();
  });

  it("shows ffmpeg warning when ffmpeg is unavailable", () => {
    render(<RoomPage {...defaultProps} ffmpegAvailable={false} />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
    cleanup();
  });

  it("does not show ffmpeg warning when ffmpeg is null (unknown)", () => {
    render(<RoomPage {...defaultProps} ffmpegAvailable={null} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    cleanup();
  });

  it("shows connection quality in status bar", () => {
    render(<RoomPage {...defaultProps} />);
    expect(screen.getByText(/50ms/)).toBeInTheDocument();
    cleanup();
  });
});
