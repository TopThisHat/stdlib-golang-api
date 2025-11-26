package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a Redis client with sensible defaults
func NewRedisClient(addr, password string) *redis.Client {
	opt := &redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           0, // Use default DB
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10, // Connection pool size
		MinIdleConns: 5,  // Keep some connections warm
	}

	client := redis.NewClient(opt)

	// Test the connection immediately
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// In main.go, we’d log this and potentially fail fast
		// Here we just return the client—let the caller decide
	}

	return client
}
