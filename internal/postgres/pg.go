package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPgxPool creates a connection pool with sensible defaults
func NewPgxPool(dsn string, logg *logger.Logger) (*pgxpool.Pool, error) {
	// Give ourselves 5 seconds to connect—if it takes longer, something’s wrong
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse the DSN and configure the pool
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres DSN: %w", err)
	}

	// Tune these for your workload (these are sensible starting points)
	cfg.MaxConns = 25               // Don’t overwhelm your DB
	cfg.MinConns = 5                // Keep some warm connections
	cfg.MaxConnLifetime = time.Hour // Recycle connections periodically
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	// Actually connect
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify we can actually talk to the database
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logg.Info("postgres connection pool ready",
		"max_conns", cfg.MaxConns,
		"min_conns", cfg.MinConns)

	return pool, nil
}
