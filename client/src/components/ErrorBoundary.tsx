import { Component, type ReactNode } from "react";
import { uiLogger } from "../utils/logger";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  onReset?: () => void;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: { componentStack?: string | null }) {
    uiLogger.error("React error boundary caught:", { error: error.message, stack: errorInfo.componentStack });
  }

  handleReset = () => {
    this.setState({ hasError: false, error: null });
    this.props.onReset?.();
  };

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;
      return (
        <main className="app-shell" style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 16, minHeight: "100vh", padding: 24, textAlign: "center" }}>
          <h1 style={{ fontSize: "1.75rem", marginBottom: 4 }}>TorrSyncPlayer</h1>
          <h2 style={{ fontSize: "1.1rem", color: "#94a3b8", fontWeight: 400, marginBottom: 8 }}>Something went wrong</h2>
          <p style={{ color: "#ff8a9d", maxWidth: 480, fontSize: "0.9rem", lineHeight: 1.5 }}>
            {this.state.error?.message ?? "An unexpected error occurred."}
          </p>
          <div style={{ display: "flex", gap: 12, marginTop: 8 }}>
            <button type="button" onClick={this.handleReset} className="primary-btn">
              Try again
            </button>
            <button type="button" onClick={this.handleReload} className="secondary-btn">
              Reload app
            </button>
          </div>
          <p style={{ color: "#64748b", fontSize: "0.8rem", marginTop: 16 }}>
            If the problem persists, please restart the application.
          </p>
        </main>
      );
    }
    return this.props.children;
  }
}

interface RoomErrorBoundaryProps {
  children: ReactNode;
  onReturnHome: () => void;
}

export function RoomErrorBoundary({ children, onReturnHome }: RoomErrorBoundaryProps) {
  return (
    <ErrorBoundary
      onReset={onReturnHome}
      fallback={
        <div className="app-shell" style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 16, minHeight: "60vh", padding: 24, textAlign: "center" }}>
          <h2 style={{ fontSize: "1.1rem", fontWeight: 400 }}>Something went wrong in the room</h2>
          <p style={{ color: "#ff8a9d", maxWidth: 480, fontSize: "0.9rem" }}>
            An error occurred. You can return to the home page and try again.
          </p>
          <button type="button" onClick={onReturnHome} className="primary-btn">
            Return to Home
          </button>
          <button type="button" onClick={() => window.location.reload()} className="secondary-btn">
            Reload app
          </button>
        </div>
      }
    >
      {children}
    </ErrorBoundary>
  );
}

export default ErrorBoundary;
