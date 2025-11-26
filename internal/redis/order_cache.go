package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/redis/go-redis/v9"
)

// Ensure OrderCache implements domain.OrderCache at compile time
var _ domain.OrderCache = (*OrderCache)(nil)

// OrderCache is a Redis implementation of domain.OrderCache
type OrderCache struct {
	client *redis.Client
	ttl    time.Duration // How long to cache entries
}

// NewOrderCache creates a Redis-backed order cache
func NewOrderCache(c *redis.Client) domain.OrderCache {
	return &OrderCache{
		client: c,
		ttl:    10 * time.Minute, // Cache orders for 10 minutes
	}
}

// Get retrieves a cached order by ID
func (c *OrderCache) Get(ctx context.Context, orderID string) (*domain.Order, error) {
	key := fmt.Sprintf("order:%s", orderID)

	data, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, domain.ErrCacheMiss // Cache miss, not an error
		}
		return nil, fmt.Errorf("redis get failed: %w", err)
	}

	var order domain.Order
	if err := json.Unmarshal([]byte(data), &order); err != nil {
		return nil, fmt.Errorf("failed to unmarshal order: %w", err)
	}

	return &order, nil
}

// Set caches an order
func (c *OrderCache) Set(ctx context.Context, order *domain.Order) error {
	key := fmt.Sprintf("order:%s", order.ID)

	data, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("failed to marshal order: %w", err)
	}

	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// Invalidate removes an order from cache (call this when updating/deleting)
func (c *OrderCache) Invalidate(ctx context.Context, orderID string) error {
	key := fmt.Sprintf("order:%s", orderID)
	return c.client.Del(ctx, key).Err()
}

// InvalidateByUserID removes all cached orders for a specific user
// This is useful when a user's orders change and you want to clear their order cache
func (c *OrderCache) InvalidateByUserID(ctx context.Context, userID string) error {
	// Use Redis SCAN to find all order keys for this user
	// Note: This requires scanning all order keys and checking userID
	// For better performance, consider maintaining a separate index set
	pattern := "order:*"
	var cursor uint64
	var keysToDelete []string

	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}

		// Check each key to see if it belongs to this user
		for _, key := range keys {
			data, err := c.client.Get(ctx, key).Result()
			if err != nil {
				continue // Skip if we can't read the key
			}

			var order domain.Order
			if err := json.Unmarshal([]byte(data), &order); err != nil {
				continue // Skip if we can't unmarshal
			}

			if order.UserID == userID {
				keysToDelete = append(keysToDelete, key)
			}
		}

		if cursor == 0 {
			break
		}
	}

	// Delete all matching keys
	if len(keysToDelete) > 0 {
		if err := c.client.Del(ctx, keysToDelete...).Err(); err != nil {
			return fmt.Errorf("failed to delete keys: %w", err)
		}
	}

	return nil
}

// GetUserOrders retrieves cached orders for a user from a user-specific index
// This is more efficient than InvalidateByUserID for retrieving user orders
// Note: This requires maintaining a separate set of order IDs per user
func (c *OrderCache) GetUserOrderIDs(ctx context.Context, userID string) ([]string, error) {
	key := fmt.Sprintf("user:%s:orders", userID)

	orderIDs, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to get user order IDs: %w", err)
	}

	return orderIDs, nil
}

// AddUserOrderIndex adds an order ID to a user's order index set
// Call this when caching an order to maintain the user-to-orders mapping
func (c *OrderCache) AddUserOrderIndex(ctx context.Context, userID, orderID string) error {
	key := fmt.Sprintf("user:%s:orders", userID)

	if err := c.client.SAdd(ctx, key, orderID).Err(); err != nil {
		return fmt.Errorf("failed to add order to user index: %w", err)
	}

	// Set TTL on the index set (same as order TTL)
	if err := c.client.Expire(ctx, key, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to set TTL on user order index: %w", err)
	}

	return nil
}

// RemoveUserOrderIndex removes an order ID from a user's order index set
// Call this when invalidating an order
func (c *OrderCache) RemoveUserOrderIndex(ctx context.Context, userID, orderID string) error {
	key := fmt.Sprintf("user:%s:orders", userID)
	return c.client.SRem(ctx, key, orderID).Err()
}
