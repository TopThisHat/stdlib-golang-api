package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration loaded from environment variables
type Config struct {
	// Application
	Environment string // "development", "staging", "production"
	Version     string
	Port        string
	LogLevel    string // "debug", "info", "warn", "error"

	// Database
	PostgresDSN         string
	PostgresMaxConns    int
	PostgresMinConns    int
	PostgresMaxIdleTime time.Duration

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// AWS
	AWSRegion          string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	S3Bucket           string

	// HTTP Server
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	// Security
	JWTSecret            string
	JWTExpirationHours   int
	AllowedOrigins       []string
	RateLimitPerMinute   int
	EnableCORS           bool
	EnableAuthentication bool

	// Feature Flags
	EnableMetrics      bool
	EnableHealthChecks bool
	EnableSwagger      bool
}

// LoadFromEnv loads configuration from environment variables with validation
// Fails fast if required variables are missing or invalid
func LoadFromEnv() *Config {
	cfg := &Config{
		// Application defaults
		Environment: getEnv("ENVIRONMENT", "development"),
		Version:     getEnv("VERSION", "0.0.0-dev"),
		Port:        getEnv("PORT", "8080"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),

		// Database
		PostgresDSN:         requireEnv("POSTGRES_DSN"),
		PostgresMaxConns:    getEnvAsInt("POSTGRES_MAX_CONNS", 25),
		PostgresMinConns:    getEnvAsInt("POSTGRES_MIN_CONNS", 5),
		PostgresMaxIdleTime: getEnvAsDuration("POSTGRES_MAX_IDLE_TIME", 15*time.Minute),

		// Redis
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvAsInt("REDIS_DB", 0),

		// AWS
		AWSRegion:          getEnv("AWS_REGION", "us-east-1"),
		AWSAccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", ""),
		AWSSecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		S3Bucket:           getEnv("S3_BUCKET", ""),

		// HTTP Server
		ReadTimeout:  getEnvAsDuration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout: getEnvAsDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:  getEnvAsDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),

		// Security
		JWTSecret:            requireEnv("JWT_SECRET"),
		JWTExpirationHours:   getEnvAsInt("JWT_EXPIRATION_HOURS", 24),
		AllowedOrigins:       getEnvAsSlice("ALLOWED_ORIGINS", []string{"*"}),
		RateLimitPerMinute:   getEnvAsInt("RATE_LIMIT_PER_MINUTE", 100),
		EnableCORS:           getEnvAsBool("ENABLE_CORS", true),
		EnableAuthentication: getEnvAsBool("ENABLE_AUTHENTICATION", true),

		// Feature Flags
		EnableMetrics:      getEnvAsBool("ENABLE_METRICS", true),
		EnableHealthChecks: getEnvAsBool("ENABLE_HEALTH_CHECKS", true),
		EnableSwagger:      getEnvAsBool("ENABLE_SWAGGER", false),
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("invalid configuration: %v", err))
	}

	return cfg
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	// Validate environment
	validEnvs := map[string]bool{
		"development": true,
		"staging":     true,
		"production":  true,
		"test":        true,
	}
	if !validEnvs[c.Environment] {
		return fmt.Errorf("invalid environment: %s (must be development, staging, production, or test)", c.Environment)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.LogLevel)
	}

	// Validate port
	if c.Port == "" {
		return fmt.Errorf("PORT cannot be empty")
	}
	if port, err := strconv.Atoi(c.Port); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid PORT: %s (must be 1-65535)", c.Port)
	}

	// Validate database config
	if c.PostgresDSN == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	if c.PostgresMaxConns < c.PostgresMinConns {
		return fmt.Errorf("POSTGRES_MAX_CONNS (%d) must be >= POSTGRES_MIN_CONNS (%d)", c.PostgresMaxConns, c.PostgresMinConns)
	}

	// Validate JWT config
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters long for security")
	}

	// Production-specific validations
	if c.Environment == "production" {
		if c.LogLevel == "debug" {
			return fmt.Errorf("debug log level should not be used in production")
		}
		if c.EnableSwagger {
			return fmt.Errorf("swagger should be disabled in production")
		}
		if contains(c.AllowedOrigins, "*") {
			return fmt.Errorf("wildcard CORS origins (*) should not be used in production")
		}
	}

	return nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}

// IsTest returns true if running in test mode
func (c *Config) IsTest() bool {
	return c.Environment == "test"
}

// DatabaseURL returns the formatted database URL for display (with password masked)
func (c *Config) DatabaseURL() string {
	return maskPassword(c.PostgresDSN)
}

// Helper functions for environment variable parsing

// getEnv reads an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// requireEnv reads an environment variable and panics if it's not set
func requireEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return value
}

// getEnvAsInt reads an environment variable as an integer or returns a default
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		panic(fmt.Sprintf("invalid integer value for %s: %s", key, valueStr))
	}
	return value
}

// getEnvAsBool reads an environment variable as a boolean or returns a default
func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		panic(fmt.Sprintf("invalid boolean value for %s: %s (use true/false, 1/0, yes/no)", key, valueStr))
	}
	return value
}

// getEnvAsDuration reads an environment variable as a duration or returns a default
func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		panic(fmt.Sprintf("invalid duration value for %s: %s (use format like '30s', '5m', '1h')", key, valueStr))
	}
	return value
}

// getEnvAsSlice reads an environment variable as a comma-separated slice or returns a default
func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	values := strings.Split(valueStr, ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return values
}

// Utility functions

// maskPassword masks the password in a connection string for safe logging
func maskPassword(dsn string) string {
	// Simple masking for postgres://user:password@host/db
	if strings.Contains(dsn, "://") && strings.Contains(dsn, "@") {
		parts := strings.SplitN(dsn, "@", 2)
		if len(parts) == 2 {
			userPass := strings.SplitN(parts[0], "://", 2)
			if len(userPass) == 2 {
				credentials := strings.SplitN(userPass[1], ":", 2)
				if len(credentials) == 2 {
					return fmt.Sprintf("%s://%s:****@%s", userPass[0], credentials[0], parts[1])
				}
			}
		}
	}
	return dsn
}

// contains checks if a string slice contains a specific value
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
