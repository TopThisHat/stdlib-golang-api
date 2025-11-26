package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	// Set required environment variables
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/testdb")
	os.Setenv("JWT_SECRET", "this-is-a-test-secret-key-with-32-chars-minimum")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
	}()

	cfg := LoadFromEnv()

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Check defaults
	if cfg.Environment != "development" {
		t.Errorf("expected environment=development, got %s", cfg.Environment)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected port=8080, got %s", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected log level=info, got %s", cfg.LogLevel)
	}
}

func TestLoadFromEnvWithCustomValues(t *testing.T) {
	// Set custom environment variables
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("PORT", "3000")
	os.Setenv("LOG_LEVEL", "warn")
	os.Setenv("POSTGRES_DSN", "postgres://user:pass@localhost:5432/proddb")
	os.Setenv("JWT_SECRET", "super-secret-jwt-key-for-production-use-only-32-chars")
	os.Setenv("POSTGRES_MAX_CONNS", "50")
	os.Setenv("ALLOWED_ORIGINS", "https://example.com,https://api.example.com")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("PORT")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("POSTGRES_MAX_CONNS")
		os.Unsetenv("ALLOWED_ORIGINS")
	}()

	cfg := LoadFromEnv()

	if cfg.Environment != "production" {
		t.Errorf("expected environment=production, got %s", cfg.Environment)
	}
	if cfg.Port != "3000" {
		t.Errorf("expected port=3000, got %s", cfg.Port)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("expected log level=warn, got %s", cfg.LogLevel)
	}
	if cfg.PostgresMaxConns != 50 {
		t.Errorf("expected max conns=50, got %d", cfg.PostgresMaxConns)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Errorf("expected 2 allowed origins, got %d", len(cfg.AllowedOrigins))
	}
}

func TestRequireEnvPanics(t *testing.T) {
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("JWT_SECRET")

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when required env var is missing")
		}
	}()

	LoadFromEnv()
}

func TestValidateEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		wantErr     bool
	}{
		{"valid development", "development", false},
		{"valid staging", "staging", false},
		{"valid production", "production", false},
		{"valid test", "test", false},
		{"invalid environment", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Environment:    tt.environment,
				Port:           "8080",
				LogLevel:       "info",
				PostgresDSN:    "postgres://user:pass@localhost/db",
				JWTSecret:      "this-is-a-test-secret-key-with-32-chars-minimum",
				AllowedOrigins: []string{"http://localhost"},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		wantErr  bool
	}{
		{"valid debug", "debug", false},
		{"valid info", "info", false},
		{"valid warn", "warn", false},
		{"valid error", "error", false},
		{"invalid level", "trace", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Environment:    "development",
				Port:           "8080",
				LogLevel:       tt.logLevel,
				PostgresDSN:    "postgres://user:pass@localhost/db",
				JWTSecret:      "this-is-a-test-secret-key-with-32-chars-minimum",
				AllowedOrigins: []string{"http://localhost"},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{"valid port 80", "80", false},
		{"valid port 8080", "8080", false},
		{"valid port 65535", "65535", false},
		{"invalid port 0", "0", true},
		{"invalid port 65536", "65536", true},
		{"invalid port empty", "", true},
		{"invalid port non-numeric", "abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Environment:    "development",
				Port:           tt.port,
				LogLevel:       "info",
				PostgresDSN:    "postgres://user:pass@localhost/db",
				JWTSecret:      "this-is-a-test-secret-key-with-32-chars-minimum",
				AllowedOrigins: []string{"http://localhost"},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateJWTSecret(t *testing.T) {
	tests := []struct {
		name      string
		jwtSecret string
		wantErr   bool
	}{
		{"valid 32 chars", "12345678901234567890123456789012", false},
		{"valid 64 chars", "1234567890123456789012345678901234567890123456789012345678901234", false},
		{"invalid empty", "", true},
		{"invalid too short", "short", true},
		{"invalid 31 chars", "1234567890123456789012345678901", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Environment:    "development",
				Port:           "8080",
				LogLevel:       "info",
				PostgresDSN:    "postgres://user:pass@localhost/db",
				JWTSecret:      tt.jwtSecret,
				AllowedOrigins: []string{"http://localhost"},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProductionConfig(t *testing.T) {
	tests := []struct {
		name           string
		logLevel       string
		enableSwagger  bool
		allowedOrigins []string
		wantErr        bool
	}{
		{"valid production", "info", false, []string{"https://example.com"}, false},
		{"invalid debug in prod", "debug", false, []string{"https://example.com"}, true},
		{"invalid swagger in prod", "info", true, []string{"https://example.com"}, true},
		{"invalid wildcard cors in prod", "info", false, []string{"*"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Environment:    "production",
				Port:           "8080",
				LogLevel:       tt.logLevel,
				PostgresDSN:    "postgres://user:pass@localhost/db",
				JWTSecret:      "this-is-a-test-secret-key-with-32-chars-minimum",
				EnableSwagger:  tt.enableSwagger,
				AllowedOrigins: tt.allowedOrigins,
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsDevelopment(t *testing.T) {
	cfg := &Config{Environment: "development"}
	if !cfg.IsDevelopment() {
		t.Error("expected IsDevelopment() to return true")
	}

	cfg.Environment = "production"
	if cfg.IsDevelopment() {
		t.Error("expected IsDevelopment() to return false")
	}
}

func TestIsProduction(t *testing.T) {
	cfg := &Config{Environment: "production"}
	if !cfg.IsProduction() {
		t.Error("expected IsProduction() to return true")
	}

	cfg.Environment = "development"
	if cfg.IsProduction() {
		t.Error("expected IsProduction() to return false")
	}
}

func TestIsTest(t *testing.T) {
	cfg := &Config{Environment: "test"}
	if !cfg.IsTest() {
		t.Error("expected IsTest() to return true")
	}

	cfg.Environment = "production"
	if cfg.IsTest() {
		t.Error("expected IsTest() to return false")
	}
}

func TestDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "postgres with password",
			dsn:      "postgres://user:secretpass@localhost:5432/mydb",
			expected: "postgres://user:****@localhost:5432/mydb",
		},
		{
			name:     "postgres without password",
			dsn:      "postgres://user@localhost:5432/mydb",
			expected: "postgres://user@localhost:5432/mydb",
		},
		{
			name:     "plain connection string",
			dsn:      "host=localhost port=5432 user=user password=pass dbname=mydb",
			expected: "host=localhost port=5432 user=user password=pass dbname=mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{PostgresDSN: tt.dsn}
			result := cfg.DatabaseURL()
			if result != tt.expected {
				t.Errorf("DatabaseURL() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGetEnvAsInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	result := getEnvAsInt("TEST_INT", 10)
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}

	result = getEnvAsInt("NONEXISTENT", 10)
	if result != 10 {
		t.Errorf("expected default 10, got %d", result)
	}
}

func TestGetEnvAsBool(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"0", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv("TEST_BOOL", tt.value)
			defer os.Unsetenv("TEST_BOOL")

			result := getEnvAsBool("TEST_BOOL", false)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetEnvAsDuration(t *testing.T) {
	os.Setenv("TEST_DURATION", "30s")
	defer os.Unsetenv("TEST_DURATION")

	result := getEnvAsDuration("TEST_DURATION", 10*time.Second)
	if result != 30*time.Second {
		t.Errorf("expected 30s, got %v", result)
	}

	result = getEnvAsDuration("NONEXISTENT", 10*time.Second)
	if result != 10*time.Second {
		t.Errorf("expected default 10s, got %v", result)
	}
}

func TestGetEnvAsSlice(t *testing.T) {
	os.Setenv("TEST_SLICE", "a,b,c,d")
	defer os.Unsetenv("TEST_SLICE")

	result := getEnvAsSlice("TEST_SLICE", []string{"default"})
	if len(result) != 4 {
		t.Errorf("expected 4 elements, got %d", len(result))
	}
	if result[0] != "a" || result[3] != "d" {
		t.Errorf("unexpected slice values: %v", result)
	}

	result = getEnvAsSlice("NONEXISTENT", []string{"default"})
	if len(result) != 1 || result[0] != "default" {
		t.Errorf("expected default slice, got %v", result)
	}
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	if !contains(slice, "banana") {
		t.Error("expected contains() to return true for 'banana'")
	}

	if contains(slice, "orange") {
		t.Error("expected contains() to return false for 'orange'")
	}
}
