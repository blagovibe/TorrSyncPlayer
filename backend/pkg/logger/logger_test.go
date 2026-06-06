package logger

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit_JSON(t *testing.T) {
	Init("info", "json")
	logger := Get()
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.Logger)
}

func TestInit_Text(t *testing.T) {
	Init("debug", "text")
	logger := Get()
	assert.NotNil(t, logger)
}

func TestInit_InvalidLevel(t *testing.T) {
	Init("invalid", "text")
	logger := Get()
	assert.NotNil(t, logger)
}

func TestInit_InvalidFormat(t *testing.T) {
	Init("info", "xml")
	logger := Get()
	assert.NotNil(t, logger)
}

func TestLoggerLevels(t *testing.T) {
	logger := &Logger{Logger: slog.New(slog.NewTextHandler(os.Stdout, nil))}
	assert.NotNil(t, logger)
	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")
}
