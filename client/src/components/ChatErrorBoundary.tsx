import { type ReactNode } from "react";
import ErrorBoundary from "./ErrorBoundary";

interface ChatErrorBoundaryProps {
  children: ReactNode;
}

export function ChatErrorBoundary({ children }: ChatErrorBoundaryProps) {
  return (
    <ErrorBoundary
      fallback={
        <div className="chat-error-fallback">
          <p className="hint">Chat is temporarily unavailable.</p>
        </div>
      }
    >
      {children}
    </ErrorBoundary>
  );
}

export default ChatErrorBoundary;
