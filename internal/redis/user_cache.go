package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/redis/go-redis/v9"
)

// Ensure UserCache implements domain.UserCache at compile time
var _ domain.UserCache = (*UserCache)(nil)

// UserCache is a Redis implementation of domain.UserCache
type UserCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewUserCache creates a Redis-backed user cache
func NewUserCache(c *redis.Client) domain.UserCache {
	return &UserCache{
		client: c,
		ttl:    5 * time.Minute,
	}
}

func (c *UserCache) Get(ctx context.Context, userID string) (*domain.User, error) {
	key := fmt.Sprintf("user:%s", userID)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, domain.ErrCacheMiss
		}
		return nil, fmt.Errorf("redis get failed: %w", err)
	}

	var user domain.User
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

func (c *UserCache) Set(ctx context.Context, user *domain.User) error {
	key := fmt.Sprintf("user:%s", user.ID)

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

func (c *UserCache) Invalidate(ctx context.Context, userID string) error {
	key := fmt.Sprintf("user:%s", userID)
	return c.client.Del(ctx, key).Err()
}
