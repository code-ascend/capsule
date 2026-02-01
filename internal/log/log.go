package log

import (
	"context"
	"log/slog"
	"os"
)

var logger *slog.Logger

func init() {
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Init initializes the global logger with the specified verbosity level.
func Init(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// Debug logs a debug message (only shown when verbose mode is enabled).
func Debug(msg string, args ...any) {
	logger.Debug(msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...any) {
	logger.Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	logger.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	logger.Error(msg, args...)
}

// With returns a logger with additional context attributes.
func With(args ...any) *slog.Logger {
	return logger.With(args...)
}

// Logger returns the underlying slog.Logger for advanced usage.
func Logger() *slog.Logger {
	return logger
}

// IsDebug returns true if debug logging is enabled.
func IsDebug() bool {
	return logger.Enabled(context.Background(), slog.LevelDebug)
}
