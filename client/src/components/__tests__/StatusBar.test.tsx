// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import StatusBar from "../StatusBar";

describe("StatusBar", () => {
  const defaultProps = {
    isConnected: true,
    torrentPeerCount: 5,
    downloadSpeed: "1.5 MB/s",
    bufferingProgress: 75,
    torrentPeerHint: "5 peers discovered",
    bufferHint: "Buffering...",
    trackerLost: false,
    connectionQuality: "good" as const,
    rttMs: 50,
  };

  beforeEach(() => {
    cleanup();
  });

  it("renders connected status", () => {
    render(<StatusBar {...defaultProps} />);
    expect(screen.getByText("connected")).toBeTruthy();
  });

  it("renders disconnected status", () => {
    render(<StatusBar {...defaultProps} isConnected={false} />);
    expect(screen.getByText("disconnected")).toBeTruthy();
  });

  it("renders peer count", () => {
    render(<StatusBar {...defaultProps} />);
    expect(screen.getByText(/Public peers seen:/)).toBeTruthy();
  });

  it("renders download speed", () => {
    render(<StatusBar {...defaultProps} />);
    expect(screen.getByText(/Speed: 1.5 MB\/s/)).toBeTruthy();
  });

  it("renders buffering progress", () => {
    render(<StatusBar {...defaultProps} />);
    const progressbar = screen.getByRole("progressbar");
    expect(progressbar.getAttribute("aria-valuenow")).toBe("75");
  });

  it("renders latency and quality", () => {
    render(<StatusBar {...defaultProps} />);
    expect(screen.getByText(/50ms/)).toBeTruthy();
    expect(screen.getByText(/good/)).toBeTruthy();
  });

  it("renders unknown quality when not measured", () => {
    render(<StatusBar {...defaultProps} connectionQuality="unknown" rttMs={null} />);
    expect(screen.getByText(/no data/)).toBeTruthy();
  });

  it("renders tracker lost warning", () => {
    render(<StatusBar {...defaultProps} trackerLost />);
    expect(screen.getByText(/Lost connection to public peers/)).toBeTruthy();
  });

  it("renders peer and buffer hints", () => {
    render(<StatusBar {...defaultProps} />);
    expect(screen.getByText("5 peers discovered")).toBeTruthy();
    expect(screen.getByText("Buffering...")).toBeTruthy();
  });

  it("clamps progress to 0-100 range", () => {
    const { rerender } = render(<StatusBar {...defaultProps} bufferingProgress={150} />);
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("100");
    rerender(<StatusBar {...defaultProps} bufferingProgress={-10} />);
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("0");
  });

  it("handles non-finite progress", () => {
    render(<StatusBar {...defaultProps} bufferingProgress={NaN} />);
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("0");
  });
});
