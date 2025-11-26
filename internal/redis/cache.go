package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/redis/go-redis/v9"
)

// Cache provides common caching operations
type Cache struct {
	client *redis.Client
}

// NewCache creates a new Cache instance
func NewCache(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// Get retrieves a value from cache and unmarshals it into the provided interface
func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return domain.ErrCacheMiss
		}
		return fmt.Errorf("redis get failed: %w", err)
	}

	if err := json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return nil
}

// Set marshals and stores a value in cache with the specified TTL
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// Delete removes a key from cache
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists in cache
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists failed: %w", err)
	}
	return count > 0, nil
}

// SetNX sets a key only if it doesn't exist (useful for distributed locks)
func (c *Cache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal data: %w", err)
	}

	result, err := c.client.SetNX(ctx, key, data, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis setnx failed: %w", err)
	}

	return result, nil
}

// Expire sets a TTL on an existing key
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// TTL returns the remaining time to live of a key
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	duration, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis ttl failed: %w", err)
	}
	return duration, nil
}

// Increment increments a numeric value stored at key
func (c *Cache) Increment(ctx context.Context, key string) (int64, error) {
	val, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr failed: %w", err)
	}
	return val, nil
}

// IncrementBy increments a numeric value by the specified amount
func (c *Cache) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	val, err := c.client.IncrBy(ctx, key, value).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incrby failed: %w", err)
	}
	return val, nil
}

// SAdd adds members to a set
func (c *Cache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SAdd(ctx, key, members...).Err()
}

// SMembers returns all members of a set
func (c *Cache) SMembers(ctx context.Context, key string) ([]string, error) {
	members, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return []string{}, nil
		}
		return nil, fmt.Errorf("redis smembers failed: %w", err)
	}
	return members, nil
}

// SRem removes members from a set
func (c *Cache) SRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SRem(ctx, key, members...).Err()
}

// FlushPattern deletes all keys matching a pattern (use with caution!)
func (c *Cache) FlushPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	var keysToDelete []string

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}

		keysToDelete = append(keysToDelete, keys...)

		if cursor == 0 {
			break
		}
	}

	if len(keysToDelete) > 0 {
		if err := c.client.Del(ctx, keysToDelete...).Err(); err != nil {
			return fmt.Errorf("failed to delete keys: %w", err)
		}
	}

	return nil
}

// Ping checks if the Redis connection is alive
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
