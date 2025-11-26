package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/config"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/TopThisHat/stdlib-golang-api/internal/postgres"
	"github.com/TopThisHat/stdlib-golang-api/internal/redis"
	"github.com/TopThisHat/stdlib-golang-api/internal/repository"
	transporthttp "github.com/TopThisHat/stdlib-golang-api/internal/transport/http"
	"github.com/TopThisHat/stdlib-golang-api/internal/usecase"
)

func main() {
	// ═══════════════════════════════════════════════
	// Phase 1: Load Configuration
	// ═══════════════════════════════════════════════
	// Read from environment, validate, fail fast if anything’s missing
	cfg := config.LoadFromEnv()

	// ═══════════════════════════════════════════════
	// Phase 2: Setup Observability
	// ═══════════════════════════════════════════════
	// Get logging working BEFORE everything else—you’ll need it
	logg := logger.New(cfg.LogLevel)
	logg.Info("starting application", "version", cfg.Version, "env", cfg.Environment)

	// ═══════════════════════════════════════════════
	// Phase 3: Initialize Infrastructure (Databases, Caches, External Services)
	// ═══════════════════════════════════════════════

	// PostgreSQL connection pool (pgx v5)
	pgPool, err := postgres.NewPgxPool(cfg.PostgresDSN, logg)
	if err != nil {
		log.Fatalf("💥 failed to connect to postgres: %v", err)
	}
	defer pgPool.Close()
	logg.Info("✓ postgres connection pool established")

	// Redis client for caching
	redisClient := redis.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword)
	defer redisClient.Close()
	logg.Info("✓ redis client initialized", "addr", cfg.RedisAddr)

	// ═══════════════════════════════════════════════
	// Phase 4: Build Dependency Graph (Repositories → Caches → Services → Handlers)
	// ═══════════════════════════════════════════════

	// Repositories (adapters implementing our interfaces)
	userRepo := repository.NewUserRepo(pgPool, logg)
	orderRepo := repository.NewOrderRepo(pgPool, logg)

	// Caches (Redis-backed cache implementations)
	userCache := redis.NewUserCache(redisClient)
	orderCache := redis.NewOrderCache(redisClient)

	// Use-cases (business logic orchestrators with cache integration)
	userSvc := usecase.NewUserService(userRepo, userCache, logg)
	orderSvc := usecase.NewOrderService(orderRepo, userRepo, orderCache, logg)

	// HTTP handlers (transport layer)
	userHandler := transporthttp.NewUserHandler(userSvc, logg)
	orderHandler := transporthttp.NewOrderHandler(orderSvc, logg)

	logg.Info("✓ services initialized",
		"user_service", "ready",
		"order_service", "ready")

	// ═══════════════════════════════════════════════
	// Phase 5: Setup HTTP Transport with Middleware
	// ═══════════════════════════════════════════════

	// Configure router with middleware stack
	routerConfig := transporthttp.RouterConfig{
		Logger:             logg,
		EnableCORS:         cfg.EnableCORS,
		AllowedOrigins:     cfg.AllowedOrigins,
		RateLimitPerMinute: cfg.RateLimitPerMinute,
		RequestTimeout:     cfg.WriteTimeout,
		MaxBodySize:        1 << 20, // 1 MB
	}

	// Create router with all middleware applied
	router := transporthttp.NewRouter(routerConfig, userHandler, orderHandler)

	// Create the HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	logg.Info("✓ middleware stack configured",
		"cors", cfg.EnableCORS,
		"rate_limit", cfg.RateLimitPerMinute,
	)

	// ═══════════════════════════════════════════════
	// Phase 6: Start Server with Graceful Shutdown
	// ═══════════════════════════════════════════════

	// Channel to listen for interrupt signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start HTTP server in a goroutine so it doesn’t block
	go func() {
		logg.Info("🚀 server starting", "addr", srv.Addr, "env", cfg.Environment)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("💥 server failed to start: %v", err)
		}
	}()

	// Block until we receive a signal (Ctrl+C or SIGTERM from orchestrator)
	<-stop
	logg.Info("🛑 shutdown signal received, draining connections...")

	// Create a context with timeout for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Gracefully shutdown: finish in-flight requests, then stop
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("💥 server shutdown failed: %v", err)
	}

	logg.Info("✓ server stopped gracefully")
}
