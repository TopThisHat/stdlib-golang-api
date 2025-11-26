package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	logger := New("info")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"warning", "WARN"},
		{"error", "ERROR"},
		{"invalid", "INFO"}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLevel(tt.input)
			if level.String() != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, level, tt.expected)
			}
		})
	}
}

func TestJSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)

	logger.Info("test message", "key", "value", "count", 42)

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected message in output: %s", output)
	}

	// Verify it's valid JSON
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, output)
	}

	// Check structured fields
	if logEntry["msg"] != "test message" {
		t.Errorf("expected msg='test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("expected key='value', got %v", logEntry["key"])
	}
	if logEntry["count"].(float64) != 42 {
		t.Errorf("expected count=42, got %v", logEntry["count"])
	}
}

func TestTextOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, false)

	logger.Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected message in output: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected key=value in output: %s", output)
	}
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		level     string
		shouldLog map[string]bool
	}{
		{
			level: "debug",
			shouldLog: map[string]bool{
				"debug": true,
				"info":  true,
				"warn":  true,
				"error": true,
			},
		},
		{
			level: "info",
			shouldLog: map[string]bool{
				"debug": false,
				"info":  true,
				"warn":  true,
				"error": true,
			},
		},
		{
			level: "warn",
			shouldLog: map[string]bool{
				"debug": false,
				"info":  false,
				"warn":  true,
				"error": true,
			},
		},
		{
			level: "error",
			shouldLog: map[string]bool{
				"debug": false,
				"info":  false,
				"warn":  false,
				"error": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewWithOptions(tt.level, &buf, false)

			logger.Debug("debug message")
			logger.Info("info message")
			logger.Warn("warn message")
			logger.Error("error message")

			output := buf.String()

			checks := map[string]string{
				"debug": "debug message",
				"info":  "info message",
				"warn":  "warn message",
				"error": "error message",
			}

			for level, msg := range checks {
				contains := strings.Contains(output, msg)
				if contains != tt.shouldLog[level] {
					t.Errorf("level=%s, message=%q: expected logged=%v, got %v\nOutput: %s",
						tt.level, msg, tt.shouldLog[level], contains, output)
				}
			}
		})
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)

	childLogger := logger.WithFields("service", "api", "version", "1.0")
	childLogger.Info("test message")

	output := buf.String()
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if logEntry["service"] != "api" {
		t.Errorf("expected service='api', got %v", logEntry["service"])
	}
	if logEntry["version"] != "1.0" {
		t.Errorf("expected version='1.0', got %v", logEntry["version"])
	}
}

func TestWithError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)

	err := errors.New("test error")
	logger.WithError(err).Info("operation failed")

	output := buf.String()
	if !strings.Contains(output, "test error") {
		t.Errorf("expected error in output: %s", output)
	}
}

func TestWithErrorNil(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)

	// Should not panic or add error field
	logger.WithError(nil).Info("operation succeeded")

	output := buf.String()
	if strings.Contains(output, "error") {
		t.Errorf("expected no error field in output: %s", output)
	}
}

func TestHTTPRequest(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantLevel  string
	}{
		{"success", 200, "INFO"},
		{"created", 201, "INFO"},
		{"client error", 400, "WARN"},
		{"not found", 404, "WARN"},
		{"server error", 500, "ERROR"},
		{"service unavailable", 503, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewWithOptions("debug", &buf, false)

			logger.HTTPRequest("GET", "/api/users", tt.statusCode, 150*time.Millisecond)

			output := buf.String()
			if !strings.Contains(output, tt.wantLevel) {
				t.Errorf("expected level %s in output: %s", tt.wantLevel, output)
			}
			if !strings.Contains(output, "GET") {
				t.Errorf("expected method in output: %s", output)
			}
			if !strings.Contains(output, "/api/users") {
				t.Errorf("expected path in output: %s", output)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)

	reqLogger := logger.Middleware("req-123", "user_id", "user-456")
	reqLogger.Info("processing request")

	output := buf.String()
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if logEntry["request_id"] != "req-123" {
		t.Errorf("expected request_id='req-123', got %v", logEntry["request_id"])
	}
	if logEntry["user_id"] != "user-456" {
		t.Errorf("expected user_id='user-456', got %v", logEntry["user_id"])
	}
}

func BenchmarkLogger(b *testing.B) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", "iteration", i, "key", "value")
	}
}

func BenchmarkLoggerWithFields(b *testing.B) {
	var buf bytes.Buffer
	logger := NewWithOptions("info", &buf, true)
	childLogger := logger.WithFields("service", "api", "version", "1.0")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		childLogger.Info("benchmark message", "iteration", i)
	}
}
