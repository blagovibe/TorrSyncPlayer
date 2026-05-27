// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import RoomInfo from "../RoomInfo";

describe("RoomInfo", () => {
  const defaultProps = {
    peerId: "ABC123",
    peerRole: "master" as const,
    peers: [{ id: "self", name: "You", role: "master" as const, connectionState: "connected" as const }],
    isConnected: true,
    onLeaveRoom: vi.fn(),
    onRequestLeave: vi.fn(),
    onCopyPeerId: vi.fn(),
    copied: false,
    chatMessages: [],
    onSendChat: vi.fn(),
  };

  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders peer ID", () => {
    render(<RoomInfo {...defaultProps} />);
    expect(screen.getByText("ABC123")).toBeTruthy();
  });

  it("renders role", () => {
    render(<RoomInfo {...defaultProps} />);
    expect(screen.getAllByText(/master/).length).toBeGreaterThan(0);
  });

  it("renders connected status", () => {
    render(<RoomInfo {...defaultProps} />);
    expect(screen.getByText("Connected")).toBeTruthy();
  });

  it("renders disconnected status", () => {
    render(<RoomInfo {...defaultProps} isConnected={false} peerRole={"master"} />);
    expect(screen.getByText("Disconnected")).toBeTruthy();
  });

  it("renders waiting for host when slave is disconnected", () => {
    render(<RoomInfo {...defaultProps} isConnected={false} peerRole="slave" />);
    expect(screen.getByText("Waiting for host...")).toBeTruthy();
  });

  it("calls onCopyPeerId when copy button clicked", () => {
    render(<RoomInfo {...defaultProps} />);
    fireEvent.click(screen.getByText("Copy"));
    expect(defaultProps.onCopyPeerId).toHaveBeenCalled();
  });

  it("shows Copied text after copy", () => {
    render(<RoomInfo {...defaultProps} copied />);
    expect(screen.getByText("Copied!")).toBeTruthy();
  });

  it("renders leave button", () => {
    render(<RoomInfo {...defaultProps} />);
    expect(screen.getByText("Leave Room")).toBeTruthy();
  });

  it("renders peer list", () => {
    render(<RoomInfo {...defaultProps} />);
    expect(screen.getByText("Peers (1)")).toBeTruthy();
    expect(screen.getByText("You")).toBeTruthy();
  });

  it("sends chat message on form submit", () => {
    render(<RoomInfo {...defaultProps} />);
    const input = screen.getByPlaceholderText("Type a message");
    fireEvent.change(input, { target: { value: "hello" } });
    fireEvent.submit(input.closest("form")!);
    expect(defaultProps.onSendChat).toHaveBeenCalledWith("hello");
  });

  it("does not send empty chat message", () => {
    render(<RoomInfo {...defaultProps} />);
    const input = screen.getByPlaceholderText("Type a message");
    fireEvent.submit(input.closest("form")!);
    expect(defaultProps.onSendChat).not.toHaveBeenCalled();
  });

  it("renders chat messages", () => {
    render(
      <RoomInfo
        {...defaultProps}
        chatMessages={[
          { id: "1", sender: "other", text: "hi there", timestamp: 1000 },
          { id: "2", sender: "ABC123", text: "hello", timestamp: 2000 },
        ]}
      />
    );
    expect(screen.getByText("hi there")).toBeTruthy();
    expect(screen.getByText("hello")).toBeTruthy();
    expect(screen.getAllByText("You").length).toBeGreaterThan(0);
  });
});
