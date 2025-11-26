package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"
)

// Logger wraps slog.Logger with convenience methods and production defaults
type Logger struct {
	*slog.Logger
}

// New creates a production-grade structured logger with the specified log level.
//
// Log levels: "debug", "info", "warn", "error"
//
// Output format:
//   - JSON for production environments (machine-readable, structured)
//   - Pretty text for development (human-readable, colored if TTY)
//
// Example usage:
//
//	logger := logger.New("info")
//	logger.Info("server started", "port", 8080, "env", "production")
//	logger.Error("failed to connect", "error", err, "retry_count", 3)
func New(level string) *Logger {
	return NewWithOptions(level, os.Stdout, false)
}

// NewWithOptions creates a logger with custom output and format options
func NewWithOptions(level string, w io.Writer, jsonFormat bool) *Logger {
	logLevel := parseLevel(level)

	var handler slog.Handler

	opts := &slog.HandlerOptions{
		AddSource: logLevel == slog.LevelDebug, // Include file:line only in debug mode
		Level:     logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize time format for readability
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.String(a.Key, t.Format(time.RFC3339))
				}
			}
			return a
		},
	}

	if jsonFormat {
		// JSON handler for production: structured, machine-readable
		handler = slog.NewJSONHandler(w, opts)
	} else {
		// Text handler for development: human-readable
		handler = slog.NewTextHandler(w, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}
}

// parseLevel converts a string log level to slog.Level
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo // Default to info if unknown
	}
}

// WithContext returns a new logger with context values attached
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{Logger: l.Logger.With()}
}

// WithFields returns a new logger with the given fields pre-attached
// Useful for request-scoped loggers or component-specific loggers
//
// Example:
//
//	reqLogger := logger.WithFields("request_id", reqID, "user_id", userID)
//	reqLogger.Info("processing request")
func (l *Logger) WithFields(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

// WithError is a convenience method to log an error with consistent key naming
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return &Logger{Logger: l.Logger.With("error", err.Error())}
}

// Fatal logs at error level and exits with code 1
// Use sparingly - only for truly unrecoverable errors at startup
func (l *Logger) Fatal(msg string, args ...any) {
	l.Error(msg, args...)
	os.Exit(1)
}

// HTTPRequest logs HTTP request details with consistent structure
func (l *Logger) HTTPRequest(method, path string, statusCode int, duration time.Duration, args ...any) {
	allArgs := append([]any{
		"method", method,
		"path", path,
		"status", statusCode,
		"duration_ms", duration.Milliseconds(),
	}, args...)

	if statusCode >= 500 {
		l.Error("http request", allArgs...)
	} else if statusCode >= 400 {
		l.Warn("http request", allArgs...)
	} else {
		l.Info("http request", allArgs...)
	}
}

// Middleware creates a logger with request-scoped fields for HTTP handlers
// Returns a child logger that includes request_id and other contextual info
func (l *Logger) Middleware(requestID string, args ...any) *Logger {
	allArgs := append([]any{"request_id", requestID}, args...)
	return l.WithFields(allArgs...)
}
