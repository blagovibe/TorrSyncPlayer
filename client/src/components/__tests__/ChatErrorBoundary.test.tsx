// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ChatErrorBoundary } from "../ChatErrorBoundary";

describe("ChatErrorBoundary", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders children when no error", () => {
    render(
      <ChatErrorBoundary>
        <div>Chat content</div>
      </ChatErrorBoundary>
    );
    expect(screen.getByText("Chat content")).toBeTruthy();
  });

  it("renders fallback UI when child throws", () => {
    const ThrowingComponent = () => {
      throw new Error("Test error");
    };

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <ChatErrorBoundary>
        <ThrowingComponent />
      </ChatErrorBoundary>
    );

    expect(screen.getByText("Chat is temporarily unavailable.")).toBeTruthy();
    expect(screen.getByText("Retry")).toBeTruthy();

    consoleSpy.mockRestore();
  });

  it("recovers when retry button is clicked", () => {
    const ThrowingComponent = () => {
      throw new Error("Test error");
    };

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <ChatErrorBoundary>
        <ThrowingComponent />
      </ChatErrorBoundary>
    );

    expect(screen.getByText("Chat is temporarily unavailable.")).toBeTruthy();

    fireEvent.click(screen.getByText("Retry"));

    // After retry, should show the error again since the component still throws
    expect(screen.getByText("Chat is temporarily unavailable.")).toBeTruthy();

    consoleSpy.mockRestore();
  });
});
