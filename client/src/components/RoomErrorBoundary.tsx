import { type ReactNode } from "react";
import ErrorBoundary from "./ErrorBoundary";

interface RoomErrorBoundaryProps {
  children: ReactNode;
  onReturnHome: () => void;
}

export function RoomErrorBoundary({ children, onReturnHome }: RoomErrorBoundaryProps) {
  return (
    <ErrorBoundary
      onReset={onReturnHome}
      fallback={
        <div className="app-shell error-boundary-shell error-boundary-shell--room">
          <h2 className="error-boundary-subtitle error-boundary-subtitle--room">Something went wrong in the room</h2>
          <p className="error-boundary-message">
            An error occurred. You can return to home page and try again.
          </p>
          <div className="error-boundary-actions">
            <button type="button" onClick={onReturnHome} className="primary-btn">
              Return to Home
            </button>
            <button type="button" onClick={() => window.location.reload()} className="secondary-btn">
              Reload app
            </button>
          </div>
        </div>
      }
    >
      {children}
    </ErrorBoundary>
  );
}

export default RoomErrorBoundary;
