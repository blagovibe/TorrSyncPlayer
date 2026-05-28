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
  showDetails: boolean;
}

function getErrorType(error: Error | null): string {
  if (!error) return "Unknown Error";
  const message = error.message.toLowerCase();
  if (message.includes("network") || message.includes("connection") || message.includes("signaling")) {
    return "Connection Error";
  }
  if (message.includes("webrtc") || message.includes("rtc") || message.includes("peer")) {
    return "WebRTC Error";
  }
  if (message.includes("torrent") || message.includes("magnet") || message.includes("metadata")) {
    return "Torrent Error";
  }
  if (message.includes("media") || message.includes("play") || message.includes("stream")) {
    return "Media Playback Error";
  }
  if (message.includes("permission") || message.includes("denied") || message.includes("blocked")) {
    return "Permission Error";
  }
  return "Application Error";
}

function getErrorHint(error: Error | null): string {
  if (!error) return "If the problem persists, please restart the application.";
  const message = error.message.toLowerCase();
  if (message.includes("network") || message.includes("connection") || message.includes("signaling")) {
    return "Check your internet connection and try again. If using a VPN, try disabling it.";
  }
  if (message.includes("webrtc") || message.includes("rtc")) {
    return "WebRTC is required for P2P connections. Try using the Electron build or a modern browser.";
  }
  if (message.includes("torrent") || message.includes("magnet")) {
    return "Check if the magnet link or torrent file is valid. Try a different source.";
  }
  if (message.includes("media") || message.includes("play") || message.includes("stream")) {
    return "The media format may not be supported. Try a different file or check if codecs are available.";
  }
  if (message.includes("permission") || message.includes("denied") || message.includes("blocked")) {
    return "Browser permissions may be blocking this action. Check your browser settings.";
  }
  return "If the problem persists, please restart the application.";
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null, showDetails: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error, showDetails: false };
  }

  componentDidCatch(error: Error, errorInfo: { componentStack?: string | null }) {
    uiLogger.error("React error boundary caught:", {
      error: error.message,
      stack: errorInfo.componentStack,
      name: error.name
    });
  }

  handleReset = () => {
    this.setState({ hasError: false, error: null, showDetails: false });
    this.props.onReset?.();
  };

  handleReload = () => {
    window.location.reload();
  };

  handleCopyDetails = async () => {
    const { error } = this.state;
    const details = [
      `Error: ${error?.name ?? "Unknown"}`,
      `Message: ${error?.message ?? "No message"}`,
      `Stack: ${error?.stack ?? "No stack trace"}`,
      `Time: ${new Date().toISOString()}`,
      `User Agent: ${navigator.userAgent}`,
    ].join("\n");
    
    try {
      await navigator.clipboard.writeText(details);
    } catch {
      uiLogger.warn("Failed to copy error details to clipboard");
    }
  };

  toggleDetails = () => {
    this.setState((prev) => ({ ...prev, showDetails: !prev.showDetails }));
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) return this.props.fallback;
      
      const errorType = getErrorType(this.state.error);
      const errorHint = getErrorHint(this.state.error);
      
      return (
        <main className="app-shell error-boundary-shell">
          <h1 className="error-boundary-title">TorrSyncPlayer</h1>
          <h2 className="error-boundary-subtitle">{errorType}</h2>
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
          <p className="error-boundary-hint">{errorHint}</p>
          <div className="error-boundary-details-section">
            <button 
              type="button" 
              onClick={this.toggleDetails} 
              className="text-btn error-boundary-details-toggle"
            >
              {this.state.showDetails ? "Hide details" : "Show details"}
            </button>
            {this.state.showDetails && (
              <div className="error-boundary-details">
                <pre className="error-boundary-stack">
                  {this.state.error?.stack ?? "No stack trace available"}
                </pre>
                <button 
                  type="button" 
                  onClick={this.handleCopyDetails} 
                  className="secondary-btn error-boundary-copy-btn"
                >
                  Copy details
                </button>
              </div>
            )}
          </div>
        </main>
      );
    }
    return this.props.children;
  }
}

export default ErrorBoundary;
