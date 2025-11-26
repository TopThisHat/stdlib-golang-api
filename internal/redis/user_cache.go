package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
)

type UserCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewUserCache(c *redis.Client) *UserCache {
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
