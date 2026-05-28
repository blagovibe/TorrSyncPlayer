// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  createLogger,
  setLogLevel,
  addLogHandler,
  setLoggerConsole,
  getLoggerConsole,
} from "../logger";

describe("logger", () => {
  let mockConsole: { log: ReturnType<typeof vi.fn>; warn: ReturnType<typeof vi.fn>; error: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    vi.useFakeTimers();
    mockConsole = {
      log: vi.fn(),
      warn: vi.fn(),
      error: vi.fn(),
    };
    setLoggerConsole(mockConsole);
  });

  afterEach(() => {
    vi.useRealTimers();
    // Reset log level
    setLogLevel("debug");
  });

  describe("createLogger", () => {
    it("creates a logger with all required methods", () => {
      const logger = createLogger("test");

      expect(typeof logger.debug).toBe("function");
      expect(typeof logger.info).toBe("function");
      expect(typeof logger.warn).toBe("function");
      expect(typeof logger.error).toBe("function");
    });

    it("logs debug messages", () => {
      const logger = createLogger("test");
      setLogLevel("debug");

      logger.debug("Debug message");

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[DEBUG]"),
        ""
      );
    });

    it("logs info messages", () => {
      const logger = createLogger("test");

      logger.info("Info message");

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[INFO]"),
        ""
      );
    });

    it("logs warn messages", () => {
      const logger = createLogger("test");

      logger.warn("Warn message");

      expect(mockConsole.warn).toHaveBeenCalledWith(
        expect.stringContaining("[WARN]"),
        ""
      );
    });

    it("logs error messages", () => {
      const logger = createLogger("test");

      logger.error("Error message");

      expect(mockConsole.error).toHaveBeenCalledWith(
        expect.stringContaining("[ERROR]"),
        ""
      );
    });

    it("includes timestamp in log messages", () => {
      const logger = createLogger("test");
      const mockDate = new Date("2024-01-15T10:30:00.000Z");
      vi.setSystemTime(mockDate);

      logger.info("Test message");

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("2024-01-15T10:30:00.000Z"),
        ""
      );
    });

    it("includes namespace in log messages", () => {
      const logger = createLogger("my-namespace");

      logger.info("Test message");

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[my-namespace]"),
        ""
      );
    });

    it("passes data as second argument when provided", () => {
      const logger = createLogger("test");
      const data = { key: "value" };

      logger.info("Test message", data);

      // Logger passes data as second argument to console.log
      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("Test message"),
        data
      );
    });
  });

  describe("setLogLevel", () => {
    it("filters out messages below the set level", () => {
      const logger = createLogger("test");

      setLogLevel("warn");

      logger.debug("Debug message");
      logger.info("Info message");
      logger.warn("Warn message");
      logger.error("Error message");

      expect(mockConsole.log).not.toHaveBeenCalled();
      expect(mockConsole.warn).toHaveBeenCalledTimes(1);
      expect(mockConsole.error).toHaveBeenCalledTimes(1);
    });

    it("shows all messages when level is debug", () => {
      const logger = createLogger("test");

      setLogLevel("debug");

      logger.debug("Debug message");
      logger.info("Info message");
      logger.warn("Warn message");
      logger.error("Error message");

      expect(mockConsole.log).toHaveBeenCalledTimes(2);
      expect(mockConsole.warn).toHaveBeenCalledTimes(1);
      expect(mockConsole.error).toHaveBeenCalledTimes(1);
    });

    it("shows only error messages when level is error", () => {
      const logger = createLogger("test");

      setLogLevel("error");

      logger.debug("Debug message");
      logger.info("Info message");
      logger.warn("Warn message");
      logger.error("Error message");

      expect(mockConsole.log).not.toHaveBeenCalled();
      expect(mockConsole.warn).not.toHaveBeenCalled();
      expect(mockConsole.error).toHaveBeenCalledTimes(1);
    });
  });

  describe("addLogHandler", () => {
    it("calls custom handler on log messages", () => {
      const handler = vi.fn();
      const logger = createLogger("test");

      addLogHandler(handler);
      logger.info("Test message");

      expect(handler).toHaveBeenCalledWith("info", "test", "Test message", undefined);
    });

    it("passes data to custom handler", () => {
      const handler = vi.fn();
      const logger = createLogger("test");
      const data = { key: "value" };

      addLogHandler(handler);
      logger.info("Test message", data);

      expect(handler).toHaveBeenCalledWith("info", "test", "Test message", data);
    });

    it("returns unsubscribe function", () => {
      const handler = vi.fn();
      const logger = createLogger("test");

      const unsubscribe = addLogHandler(handler);
      logger.info("Test message 1");
      expect(handler).toHaveBeenCalledTimes(1);

      unsubscribe();
      logger.info("Test message 2");
      expect(handler).toHaveBeenCalledTimes(1); // Not called again
    });

    it("handles multiple handlers", () => {
      const handler1 = vi.fn();
      const handler2 = vi.fn();
      const logger = createLogger("test");

      addLogHandler(handler1);
      addLogHandler(handler2);
      logger.info("Test message");

      expect(handler1).toHaveBeenCalledTimes(1);
      expect(handler2).toHaveBeenCalledTimes(1);
    });

    it("ignores handler errors", () => {
      const errorHandler = vi.fn().mockImplementation(() => {
        throw new Error("Handler error");
      });
      const normalHandler = vi.fn();
      const logger = createLogger("test");

      addLogHandler(errorHandler);
      addLogHandler(normalHandler);

      // Should not throw
      logger.info("Test message");

      expect(normalHandler).toHaveBeenCalledTimes(1);
    });
  });

  describe("setLoggerConsole / getLoggerConsole", () => {
    it("sets custom console object", () => {
      const customConsole = {
        log: vi.fn(),
        warn: vi.fn(),
        error: vi.fn(),
      };

      setLoggerConsole(customConsole);
      const retrieved = getLoggerConsole();

      expect(retrieved).toBe(customConsole);
    });

    it("uses custom console for logging", () => {
      const customConsole = {
        log: vi.fn(),
        warn: vi.fn(),
        error: vi.fn(),
      };

      setLoggerConsole(customConsole);
      const logger = createLogger("test");

      logger.info("Test message");

      expect(customConsole.log).toHaveBeenCalled();
      expect(mockConsole.log).not.toHaveBeenCalled();
    });
  });

  describe("Edge cases", () => {
    it("handles empty message", () => {
      const logger = createLogger("test");

      logger.info("");

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[INFO]"),
        ""
      );
    });

    it("handles undefined data", () => {
      const logger = createLogger("test");

      logger.info("Test message", undefined);

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("Test message"),
        ""
      );
    });

    it("handles null data", () => {
      const logger = createLogger("test");

      logger.info("Test message", null);

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("Test message"),
        ""
      );
    });

    it("handles complex data objects", () => {
      const logger = createLogger("test");
      const complexData = {
        nested: { deep: { value: 123 } },
        array: [1, 2, 3],
        fn: () => {},
      };

      logger.info("Test message", complexData);

      expect(mockConsole.log).toHaveBeenCalled();
    });

    it("handles special characters in namespace", () => {
      const logger = createLogger("test-namespace_with.special@chars");

      logger.info("Test message");

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[test-namespace_with.special@chars]"),
        ""
      );
    });

    it("handles very long messages", () => {
      const logger = createLogger("test");
      const longMessage = "a".repeat(10000);

      logger.info(longMessage);

      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining(longMessage),
        ""
      );
    });

    it("handles concurrent loggers with different namespaces", () => {
      const logger1 = createLogger("namespace1");
      const logger2 = createLogger("namespace2");

      logger1.info("Message 1");
      logger2.info("Message 2");

      expect(mockConsole.log).toHaveBeenCalledTimes(2);
      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[namespace1]"),
        ""
      );
      expect(mockConsole.log).toHaveBeenCalledWith(
        expect.stringContaining("[namespace2]"),
        ""
      );
    });
  });
});
