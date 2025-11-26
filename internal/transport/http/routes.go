package http

import (
	"net/http"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
)

// RouterConfig holds configuration for the HTTP router
type RouterConfig struct {
	Logger             *logger.Logger
	EnableCORS         bool
	AllowedOrigins     []string
	RateLimitPerMinute int
	RequestTimeout     time.Duration
	MaxBodySize        int64 // in bytes
}

// DefaultRouterConfig returns sensible defaults
func DefaultRouterConfig(logg *logger.Logger) RouterConfig {
	return RouterConfig{
		Logger:             logg,
		EnableCORS:         true,
		AllowedOrigins:     []string{"*"},
		RateLimitPerMinute: 100,
		RequestTimeout:     30 * time.Second,
		MaxBodySize:        1 << 20, // 1 MB
	}
}

// NewRouter creates a new HTTP router with middleware stack applied
func NewRouter(config RouterConfig, userHandler *UserHandler, orderHandler *OrderHandler) http.Handler {
	mux := http.NewServeMux()

	// Register routes
	registerRoutes(mux, userHandler, orderHandler)

	// Build middleware stack (order matters - first applied is outermost)
	middlewares := []Middleware{
		// Outermost: Request ID for tracing
		RequestID(),
		// Recovery from panics
		Recover(config.Logger),
		// Request logging
		Logging(config.Logger),
		// Security headers
		SecureHeaders(),
		// Request body size limit
		MaxBodySize(config.MaxBodySize),
	}

	// Conditional middlewares
	if config.EnableCORS {
		corsConfig := DefaultCORSConfig()
		corsConfig.AllowedOrigins = config.AllowedOrigins
		middlewares = append(middlewares, CORS(corsConfig))
	}

	if config.RateLimitPerMinute > 0 {
		limiter := NewRateLimiter(config.RateLimitPerMinute, time.Minute)
		middlewares = append(middlewares, RateLimit(limiter))
	}

	// Content-Type validation for API routes
	middlewares = append(middlewares, ContentType("application/json"))

	// Apply middleware chain
	return Chain(mux, middlewares...)
}

// registerRoutes sets up all API routes on the mux
func registerRoutes(mux *http.ServeMux, userHandler *UserHandler, orderHandler *OrderHandler) {
	// Health check (no auth required)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
	})

	// Readiness check
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	// User routes
	mux.HandleFunc("POST /api/users", userHandler.Create)
	mux.HandleFunc("GET /api/users", userHandler.List)
	mux.HandleFunc("GET /api/users/{id}", userHandler.GetByID)
	mux.HandleFunc("PUT /api/users/{id}", userHandler.Update)
	mux.HandleFunc("DELETE /api/users/{id}", userHandler.Delete)

	// User's orders route
	mux.HandleFunc("GET /api/users/{user_id}/orders", orderHandler.GetByUserID)

	// Order routes
	mux.HandleFunc("POST /api/orders", orderHandler.Create)
	mux.HandleFunc("GET /api/orders", orderHandler.List)
	mux.HandleFunc("GET /api/orders/{id}", orderHandler.GetByID)

	// Order status transition routes
	mux.HandleFunc("POST /api/orders/{id}/confirm", orderHandler.Confirm)
	mux.HandleFunc("POST /api/orders/{id}/ship", orderHandler.Ship)
	mux.HandleFunc("POST /api/orders/{id}/deliver", orderHandler.Deliver)
	mux.HandleFunc("POST /api/orders/{id}/cancel", orderHandler.Cancel)
}

// RegisterRoutes is kept for backwards compatibility
// Deprecated: Use NewRouter instead
func RegisterRoutes(mux *http.ServeMux, userHandler *UserHandler, orderHandler *OrderHandler) {
	registerRoutes(mux, userHandler, orderHandler)
}
