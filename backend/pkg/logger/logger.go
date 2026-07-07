// SPDX-License-Identifier: MIT

// Package pkg provides a logger based on slog
package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Logger is a wrapper around slog.Logger
type Logger struct {
	*slog.Logger
}

var (
	defaultLogger *Logger
	initOnce      sync.Once
)

// Init initializes the global logger.
// Safe for concurrent calls - initialization occurs only once.
func Init(level string, format string) {
	initOnce.Do(func() {
		var logLevel slog.Level

		switch strings.ToLower(level) {
		case "debug":
			logLevel = slog.LevelDebug
		case "info":
			logLevel = slog.LevelInfo
		case "warn", "warning":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		default:
			logLevel = slog.LevelInfo
		}

		var handler slog.Handler
		opts := &slog.HandlerOptions{
			Level: logLevel,
		}

		if strings.ToLower(format) == "json" {
			handler = slog.NewJSONHandler(os.Stdout, opts)
		} else {
			handler = slog.NewTextHandler(os.Stdout, opts)
		}

		defaultLogger = &Logger{
			Logger: slog.New(handler),
		}
	})
}

// Get returns the global logger.
// If the logger is not initialized, creates a logger with default settings.
func Get() *Logger {
	initOnce.Do(func() {
		Init("info", "text")
	})
	return defaultLogger
}

// Debug logs a debug message
func Debug(msg string, args ...any) {
	Get().Debug(msg, args...)
}

// Info logs an informational message
func Info(msg string, args ...any) {
	Get().Info(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	Get().Warn(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...any) {
	Get().Error(msg, args...)
}
