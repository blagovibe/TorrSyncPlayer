// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import HomePage from "../HomePage";

describe("HomePage", () => {
  const defaultProps = {
    peerId: "ABC123",
    onCreateRoom: vi.fn(),
    onJoinRoom: vi.fn(),
    isConnecting: false,
    connectionError: null,
  };

  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders the app logo", () => {
    render(<HomePage {...defaultProps} />);
    expect(screen.getByText("TorrSyncPlayer")).toBeTruthy();
  });

  it("renders create room button", () => {
    render(<HomePage {...defaultProps} />);
    expect(screen.getByText("Create Room (Host)")).toBeTruthy();
  });

  it("calls onCreateRoom when create room button is clicked", () => {
    render(<HomePage {...defaultProps} />);
    fireEvent.click(screen.getByText("Create Room (Host)"));
    expect(defaultProps.onCreateRoom).toHaveBeenCalledTimes(1);
  });

  it("disables create room button when connecting", () => {
    render(<HomePage {...defaultProps} isConnecting={true} />);
    const button = screen.getByText("Creating…", { exact: false }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it("renders peer ID when available", () => {
    render(<HomePage {...defaultProps} />);
    expect(screen.getByText("ABC123")).toBeTruthy();
  });

  it("does not render peer ID when empty", () => {
    render(<HomePage {...defaultProps} peerId="" />);
    expect(screen.queryByText("ABC123")).toBeNull();
  });

  it("disables join button when code is not 6 characters", () => {
    render(<HomePage {...defaultProps} />);
    const input = screen.getByPlaceholderText("Enter friend's ID") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "ABC" } });
    const button = screen.getByText("Connect") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it("enables join button when code is 6 valid characters", () => {
    render(<HomePage {...defaultProps} />);
    const input = screen.getByPlaceholderText("Enter friend's ID") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "ABC123" } });
    const button = screen.getByText("Connect") as HTMLButtonElement;
    expect(button.disabled).toBe(false);
  });

  it("disables join button when code contains invalid characters", () => {
    render(<HomePage {...defaultProps} />);
    const input = screen.getByPlaceholderText("Enter friend's ID") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "!!!!!!" } });
    const button = screen.getByText("Connect") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it("calls onJoinRoom with uppercase code on form submit", () => {
    render(<HomePage {...defaultProps} />);
    const input = screen.getByPlaceholderText("Enter friend's ID") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "abc123" } });
    fireEvent.submit(input.closest("form")!);
    expect(defaultProps.onJoinRoom).toHaveBeenCalledWith("ABC123");
  });

  it("renders connection error when present", () => {
    render(<HomePage {...defaultProps} connectionError="Connection failed" />);
    expect(screen.getByText("Connection failed")).toBeTruthy();
  });

  it("shows error for invalid characters in join code", () => {
    render(<HomePage {...defaultProps} />);
    const input = screen.getByPlaceholderText("Enter friend's ID") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "!!!!!!" } });
    fireEvent.submit(input.closest("form")!);
    expect(screen.getByText(/only letters A-Z and digits 0-9/i)).toBeTruthy();
  });
});
