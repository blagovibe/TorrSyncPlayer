// @vitest-environment jsdom
import type { ReactElement } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { RoomErrorBoundary } from "../RoomErrorBoundary";

function ThrowingComponent(): ReactElement {
  throw new Error("Room error");
}

describe("RoomErrorBoundary", () => {
  beforeEach(() => {
    cleanup();
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  it("renders children when no error", () => {
    render(
      <RoomErrorBoundary onReturnHome={vi.fn()}>
        <div>Room content</div>
      </RoomErrorBoundary>
    );
    expect(screen.getByText("Room content")).toBeTruthy();
  });

  it("renders room error fallback when child throws", () => {
    render(
      <RoomErrorBoundary onReturnHome={vi.fn()}>
        <ThrowingComponent />
      </RoomErrorBoundary>
    );
    expect(screen.getByText("Something went wrong in the room")).toBeTruthy();
  });

  it("calls onReturnHome when Return to Home is clicked", () => {
    const onReturnHome = vi.fn();
    render(
      <RoomErrorBoundary onReturnHome={onReturnHome}>
        <ThrowingComponent />
      </RoomErrorBoundary>
    );
    fireEvent.click(screen.getByText("Return to Home"));
    expect(onReturnHome).toHaveBeenCalled();
  });
});
