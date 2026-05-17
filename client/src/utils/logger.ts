/**
 * Structured logging system.
 *
 * Replaces console.log/warn/error with leveled, namespaced logging.
 * In production, logs can be sent to a remote service or written to file.
 */

export type LogLevel = "debug" | "info" | "warn" | "error";

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

let currentLogLevel: LogLevel = "debug";
  const logHandlers: Array<(level: LogLevel, namespace: string, message: string, data?: unknown) => void> = [];

export function setLogLevel(level: LogLevel): void {
  currentLogLevel = level;
}

export function addLogHandler(
  handler: (level: LogLevel, namespace: string, message: string, data?: unknown) => void,
): () => void {
  logHandlers.push(handler);
  return () => {
    const idx = logHandlers.indexOf(handler);
    if (idx !== -1) logHandlers.splice(idx, 1);
  };
}

export interface Logger {
  debug: (message: string, data?: unknown) => void;
  info: (message: string, data?: unknown) => void;
  warn: (message: string, data?: unknown) => void;
  error: (message: string, data?: unknown) => void;
}

export function createLogger(namespace: string): Logger {
  function log(level: LogLevel, message: string, data?: unknown): void {
    if (LOG_LEVELS[level] < LOG_LEVELS[currentLogLevel]) {
      return;
    }
    const timestamp = new Date().toISOString();
    const formatted = `[${timestamp}] [${level.toUpperCase()}] [${namespace}] ${message}`;

    // Always log to console in development
    if (level === "error") {
      console.error(formatted, data ?? "");
    } else if (level === "warn") {
      console.warn(formatted, data ?? "");
    } else {
      console.log(formatted, data ?? "");
    }

    // Notify custom handlers
    for (let i = 0; i < logHandlers.length; i++) {
      try {
        logHandlers[i](level, namespace, message, data);
      } catch {
        // Ignore handler errors
      }
    }
  }

  return {
    debug: (message: string, data?: unknown) => log("debug", message, data),
    info: (message: string, data?: unknown) => log("info", message, data),
    warn: (message: string, data?: unknown) => log("warn", message, data),
    error: (message: string, data?: unknown) => log("error", message, data),
  };
}

// Application loggers
export const torrentLogger = createLogger("torrent");
export const p2pLogger = createLogger("p2p");
export const syncLogger = createLogger("sync");
export const uiLogger = createLogger("ui");
export const electronLogger = createLogger("electron");
