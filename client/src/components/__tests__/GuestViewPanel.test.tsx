// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import GuestViewPanel from "../GuestViewPanel";

describe("GuestViewPanel", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  const defaultProps = {
    sharedSourceLabel: null,
    torrentFileName: null,
    isLoadingTorrent: false,
    torrentError: null,
  };

  it("renders guest view badge", () => {
    render(<GuestViewPanel {...defaultProps} />);
    expect(screen.getByText("Guest view")).toBeTruthy();
  });

  it("shows waiting message when no source is loaded", () => {
    render(<GuestViewPanel {...defaultProps} />);
    expect(screen.getByText(/Waiting for host to load the shared source/)).toBeTruthy();
  });

  it("shows connected source label when provided", () => {
    render(<GuestViewPanel {...defaultProps} sharedSourceLabel="Test Movie.mp4" />);
    expect(screen.getByText(/Connected to: Test Movie.mp4/)).toBeTruthy();
  });

  it("shows loading indicator when loading", () => {
    render(<GuestViewPanel {...defaultProps} isLoadingTorrent={true} />);
    expect(screen.getByText("Loading torrent metadata...")).toBeTruthy();
  });

  it("shows error message when torrent error occurs", () => {
    render(<GuestViewPanel {...defaultProps} torrentError="Connection failed" />);
    expect(screen.getByText("Connection failed")).toBeTruthy();
  });

  it("shows request resend button when error occurs and handler provided", () => {
    const onRequestResend = vi.fn();
    render(<GuestViewPanel {...defaultProps} torrentError="Connection failed" onRequestResend={onRequestResend} />);
    expect(screen.getByText("Request Resend")).toBeTruthy();
  });

  it("does not show request resend button when no error", () => {
    const onRequestResend = vi.fn();
    render(<GuestViewPanel {...defaultProps} onRequestResend={onRequestResend} />);
    expect(screen.queryByText("Request Resend")).toBeNull();
  });
});
