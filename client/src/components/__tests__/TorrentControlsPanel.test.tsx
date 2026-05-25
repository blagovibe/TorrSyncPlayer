// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import TorrentControlsPanel from "../TorrentControlsPanel";

describe("TorrentControlsPanel", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  const defaultProps = {
    magnetLink: "",
    torrentFileName: null,
    sharedSourceLabel: null,
    isLoadingTorrent: false,
    torrentError: null,
    syncToleranceSeconds: 1.5,
    onMagnetLinkChange: vi.fn(),
    onTorrentFileChange: vi.fn(),
    onLoadMagnet: vi.fn(),
    onLoadTorrentFile: vi.fn(),
    onSyncToleranceChange: vi.fn(),
  };

  it("renders magnet link textarea", () => {
    render(<TorrentControlsPanel {...defaultProps} />);
    expect(screen.getByLabelText("Magnet link")).toBeTruthy();
  });

  it("renders sync tolerance input", () => {
    render(<TorrentControlsPanel {...defaultProps} />);
    const input = screen.getByLabelText("Sync tolerance, seconds") as HTMLInputElement;
    expect(input).toBeTruthy();
    expect(input.value).toBe("1.5");
  });

  it("calls onMagnetLinkChange when textarea changes", () => {
    render(<TorrentControlsPanel {...defaultProps} />);
    const textarea = screen.getByLabelText("Magnet link") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "magnet:?xt=urn:btih:abc123" } });
    expect(defaultProps.onMagnetLinkChange).toHaveBeenCalled();
  });

  it("shows validation error for invalid magnet link after change", async () => {
    render(<TorrentControlsPanel {...defaultProps} />);
    const textarea = screen.getByLabelText("Magnet link") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "invalid-magnet" } });
    expect(await screen.findByText("Invalid magnet link format")).toBeTruthy();
  });

  it("shows validation success for valid magnet link after change", async () => {
    render(<TorrentControlsPanel {...defaultProps} />);
    const textarea = screen.getByLabelText("Magnet link") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "magnet:?xt=urn:btih:abcd1234abcd1234abcd1234abcd1234abcd1234" } });
    expect(await screen.findByText("Valid magnet link format")).toBeTruthy();
  });

  it("disables load button when magnet link is empty", () => {
    render(<TorrentControlsPanel {...defaultProps} magnetLink="" />);
    const button = screen.getByText("Load Magnet") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it("enables load button when magnet link is provided", () => {
    render(<TorrentControlsPanel {...defaultProps} magnetLink="magnet:?xt=urn:btih:abcd1234abcd1234abcd1234abcd1234abcd1234" />);
    const button = screen.getByText("Load Magnet") as HTMLButtonElement;
    expect(button.disabled).toBe(false);
  });

  it("renders torrent file error when provided", () => {
    render(<TorrentControlsPanel {...defaultProps} torrentError="Failed to load torrent" />);
    expect(screen.getByText("Failed to load torrent")).toBeTruthy();
  });

  it("renders change source button when shared source label exists", () => {
    render(<TorrentControlsPanel {...defaultProps} sharedSourceLabel="test.torrent" onResetTorrentInRoom={vi.fn()} onShowResetConfirm={vi.fn()} />);
    expect(screen.getByText("Change Source")).toBeTruthy();
  });

  it("calls onSyncToleranceChange when tolerance input changes", () => {
    render(<TorrentControlsPanel {...defaultProps} />);
    const input = screen.getByLabelText("Sync tolerance, seconds") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "2.5" } });
    expect(defaultProps.onSyncToleranceChange).toHaveBeenCalledWith(2.5);
  });
});
