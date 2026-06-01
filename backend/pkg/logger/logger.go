// Package pkg предоставляет логгер на основе slog
package logger

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Logger обёртка над slog.Logger
type Logger struct {
	*slog.Logger
}

var (
	defaultLogger *Logger
	initOnce      sync.Once
)

// Init инициализирует глобальный логгер.
// Безопасен для конкурентного вызова - инициализация произойдёт только один раз.
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

// Get возвращает глобальный логгер.
// Если логгер не инициализирован, создаёт логгер с настройками по умолчанию.
func Get() *Logger {
	initOnce.Do(func() {
		Init("info", "text")
	})
	return defaultLogger
}

// Debug логирование отладочного сообщения
func Debug(msg string, args ...any) {
	Get().Logger.Debug(msg, args...)
}

// Info логирование информационного сообщения
func Info(msg string, args ...any) {
	Get().Logger.Info(msg, args...)
}

// Warn логирование предупреждения
func Warn(msg string, args ...any) {
	Get().Logger.Warn(msg, args...)
}

// Error логирование ошибки
func Error(msg string, args ...any) {
	Get().Logger.Error(msg, args...)
}
