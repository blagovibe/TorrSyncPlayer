// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import ConfirmModal from "../ConfirmModal";

describe("ConfirmModal", () => {
  const defaultProps = {
    isOpen: true,
    title: "Confirm Action",
    message: "Are you sure?",
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
  };

  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders title and message when open", () => {
    render(<ConfirmModal {...defaultProps} />);
    expect(screen.getByText("Confirm Action")).toBeTruthy();
    expect(screen.getByText("Are you sure?")).toBeTruthy();
  });

  it("does not render when closed", () => {
    const { container } = render(<ConfirmModal {...defaultProps} isOpen={false} />);
    expect(container.querySelector("dialog")).toBeNull();
  });

  it("calls onConfirm when confirm button is clicked", () => {
    render(<ConfirmModal {...defaultProps} />);
    fireEvent.click(screen.getByText("Confirm"));
    expect(defaultProps.onConfirm).toHaveBeenCalledTimes(1);
  });

  it("calls onCancel when cancel button is clicked", () => {
    render(<ConfirmModal {...defaultProps} />);
    fireEvent.click(screen.getByText("Cancel"));
    expect(defaultProps.onCancel).toHaveBeenCalledTimes(1);
  });

  it("renders custom confirm label", () => {
    render(<ConfirmModal {...defaultProps} confirmLabel="Delete" />);
    expect(screen.getByText("Delete")).toBeTruthy();
  });

  it("renders custom cancel label", () => {
    render(<ConfirmModal {...defaultProps} cancelLabel="Go Back" />);
    expect(screen.getByText("Go Back")).toBeTruthy();
  });

  it("renders danger button when danger prop is true", () => {
    const { container } = render(<ConfirmModal {...defaultProps} danger={true} />);
    expect(container.querySelector(".danger-btn")).toBeTruthy();
  });

  it("renders primary button when danger prop is false", () => {
    const { container } = render(<ConfirmModal {...defaultProps} danger={false} />);
    expect(container.querySelector(".primary-btn")).toBeTruthy();
  });
});
