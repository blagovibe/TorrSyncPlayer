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
        <main className="app-shell error-boundary-shell">
          <h1 className="error-boundary-title">TorrSyncPlayer</h1>
          <h2 className="error-boundary-subtitle">Something went wrong</h2>
          <p className="error-boundary-message">
            {this.state.error?.message ?? "An unexpected error occurred."}
          </p>
          <div className="error-boundary-actions">
            <button type="button" onClick={this.handleReset} className="primary-btn">
              Try again
            </button>
            <button type="button" onClick={this.handleReload} className="secondary-btn">
              Reload app
            </button>
          </div>
          <p className="error-boundary-hint">
            If the problem persists, please restart the application.
          </p>
        </main>
      );
    }
    return this.props.children;
  }
}

export default ErrorBoundary;
