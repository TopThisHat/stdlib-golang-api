package http

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/google/uuid"
)

// Middleware is a function that wraps an http.Handler
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares in order (first middleware wraps outermost)
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// ═══════════════════════════════════════════════════════════════════════════════
// Context Keys
// ═══════════════════════════════════════════════════════════════════════════════

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	UserIDKey    contextKey = "user_id"
)

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// ═══════════════════════════════════════════════════════════════════════════════
// Request ID Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// RequestID adds a unique request ID to each request for tracing
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for existing request ID (from load balancer/proxy)
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Add to context and response header
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			w.Header().Set("X-Request-ID", requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Logging Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Logging logs each HTTP request with timing and status
func Logging(logg *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriter(w)

			// Execute request
			next.ServeHTTP(wrapped, r)

			// Log request details
			duration := time.Since(start)
			requestID := GetRequestID(r.Context())

			logg.Info("http request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"bytes", wrapped.written,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Recovery Middleware (Panic Handler)
// ═══════════════════════════════════════════════════════════════════════════════

// Recover recovers from panics and returns a 500 error
func Recover(logg *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := GetRequestID(r.Context())
					stack := debug.Stack()

					logg.Error("panic recovered",
						"request_id", requestID,
						"error", err,
						"stack", string(stack),
						"path", r.URL.Path,
						"method", r.Method,
					)

					respondError(w, http.StatusInternalServerError,
						"INTERNAL_ERROR", "An unexpected error occurred")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CORS Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int // Preflight cache duration in seconds
}

// DefaultCORSConfig returns sensible CORS defaults
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400, // 24 hours
	}
}

// CORS handles Cross-Origin Resource Sharing
func CORS(config CORSConfig) Middleware {
	allowedOriginsMap := make(map[string]bool)
	allowAll := false
	for _, origin := range config.AllowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		allowedOriginsMap[origin] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			if allowAll || allowedOriginsMap[origin] {
				if allowAll {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", string(rune(config.MaxAge)))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Rate Limiting Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// RateLimiter implements a simple token bucket rate limiter per IP
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // requests per window
	window   time.Duration // time window
}

type visitor struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a rate limiter with the specified rate per window
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}

	// Cleanup old entries periodically
	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastReset) > rl.window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{
			tokens:    rl.rate - 1,
			lastReset: time.Now(),
		}
		return true
	}

	// Reset tokens if window has passed
	if time.Since(v.lastReset) > rl.window {
		v.tokens = rl.rate - 1
		v.lastReset = time.Now()
		return true
	}

	// Check if tokens available
	if v.tokens > 0 {
		v.tokens--
		return true
	}

	return false
}

// RateLimit middleware limits requests per IP
func RateLimit(limiter *RateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract IP (handle X-Forwarded-For for proxies)
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = strings.Split(forwarded, ",")[0]
			}

			if !limiter.allow(ip) {
				w.Header().Set("Retry-After", "60")
				respondError(w, http.StatusTooManyRequests,
					"RATE_LIMIT_EXCEEDED", "Too many requests, please try again later")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Security Headers Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// SecureHeaders adds security-related HTTP headers
func SecureHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// XSS protection (legacy but still useful)
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// Referrer policy
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy (adjust based on your needs)
			w.Header().Set("Content-Security-Policy", "default-src 'self'")

			// HSTS (only enable if using HTTPS)
			// w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

			next.ServeHTTP(w, r)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Request Timeout Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// Timeout wraps the handler with a request timeout
func Timeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Create a channel to signal completion
			done := make(chan struct{})

			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// Request completed normally
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded {
					respondError(w, http.StatusGatewayTimeout,
						"REQUEST_TIMEOUT", "Request took too long to process")
				}
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Content-Type Validation Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// ContentType ensures requests with body have correct Content-Type
func ContentType(contentTypes ...string) Middleware {
	allowedTypes := make(map[string]bool)
	for _, ct := range contentTypes {
		allowedTypes[ct] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only check for methods that typically have a body
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
				ct := r.Header.Get("Content-Type")
				// Extract media type without parameters
				mediaType := strings.Split(ct, ";")[0]
				mediaType = strings.TrimSpace(mediaType)

				if ct == "" || !allowedTypes[mediaType] {
					respondError(w, http.StatusUnsupportedMediaType,
						"UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Request Size Limiter Middleware
// ═══════════════════════════════════════════════════════════════════════════════

// MaxBodySize limits the request body size
func MaxBodySize(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
