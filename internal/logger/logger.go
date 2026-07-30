package logger

import (
	"log/slog"
	"os"
)

// Logger is the process-wide structured logger.
var Logger *slog.Logger

// Init initializes the process-wide logger.
func Init() {
	Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

// Info logs an informational message.
func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}
