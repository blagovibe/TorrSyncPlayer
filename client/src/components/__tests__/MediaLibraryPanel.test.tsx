// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import MediaLibraryPanel from "../MediaLibraryPanel";
import type { TorrentMediaFile } from "../../services/TorrentService";

describe("MediaLibraryPanel", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  const createMockFile = (index: number, name: string, kind: "video" | "audio" = "video"): TorrentMediaFile => ({
    index,
    name,
    length: 1024 * 1024 * 100,
    kind,
    extension: kind === "video" ? ".mp4" : ".mp3",
    file: {
      name,
      length: 1024 * 1024 * 100,
      streamTo: vi.fn(),
    },
  });

  const defaultProps = {
    mediaFiles: [] as TorrentMediaFile[],
    selectedMediaIndex: null as number | null,
    selectedMediaLabel: null as string | null,
    selectedMediaKind: null as "video" | "audio" | null,
    isLoadingTorrent: false,
    isHost: true,
    onSelectMediaFile: vi.fn(),
  };

  it("renders playable files header for host", () => {
    render(<MediaLibraryPanel {...defaultProps} />);
    expect(screen.getByText("Playable files")).toBeTruthy();
  });

  it("renders now playing header for guest", () => {
    render(<MediaLibraryPanel {...defaultProps} isHost={false} />);
    expect(screen.getByText("Now playing")).toBeTruthy();
  });

  it("shows empty state when no media files", () => {
    render(<MediaLibraryPanel {...defaultProps} />);
    expect(screen.getByText("No playable video or audio files found yet.")).toBeTruthy();
  });

  it("renders media files list", () => {
    const files = [createMockFile(0, "movie.mp4"), createMockFile(1, "audio.mp3", "audio")];
    render(<MediaLibraryPanel {...defaultProps} mediaFiles={files} />);
    expect(screen.getByText("movie.mp4")).toBeTruthy();
    expect(screen.getByText("audio.mp3")).toBeTruthy();
  });

  it("calls onSelectMediaFile when clicking a file", () => {
    const files = [createMockFile(0, "movie.mp4")];
    const onSelectMediaFile = vi.fn();
    render(<MediaLibraryPanel {...defaultProps} mediaFiles={files} onSelectMediaFile={onSelectMediaFile} />);
    fireEvent.click(screen.getByText("movie.mp4"));
    expect(onSelectMediaFile).toHaveBeenCalledWith(files[0]);
  });

  it("shows file sizes using formatBytes", () => {
    const files = [createMockFile(0, "movie.mp4")];
    render(<MediaLibraryPanel {...defaultProps} mediaFiles={files} />);
    expect(screen.getByText("100 MB")).toBeTruthy();
  });

  it("highlights selected media file", () => {
    const files = [createMockFile(0, "movie.mp4"), createMockFile(1, "other.mp4")];
    render(<MediaLibraryPanel {...defaultProps} mediaFiles={files} selectedMediaIndex={0} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons[0].className).toContain("active");
  });

  it("shows guest view with selected media label", () => {
    render(<MediaLibraryPanel {...defaultProps} isHost={false} selectedMediaLabel="Host Movie.mp4" selectedMediaKind="video" />);
    expect(screen.getByText("Host Movie.mp4")).toBeTruthy();
  });

  it("shows guest waiting message when no media selected", () => {
    render(<MediaLibraryPanel {...defaultProps} isHost={false} />);
    expect(screen.getByText("No file selected yet.")).toBeTruthy();
  });
});
