package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// orderRepo is the PostgreSQL implementation of domain.OrderRepository
// It contains NO business logic - only data persistence
type orderRepo struct {
	db   *pgxpool.Pool
	logg *logger.Logger
}

// NewOrderRepo creates a Postgres-backed order repository
func NewOrderRepo(db *pgxpool.Pool, logg *logger.Logger) domain.OrderRepository {
	return &orderRepo{db: db, logg: logg}
}

// GetByID fetches an order by ID
// Responsibility: Query database and translate errors to domain errors
func (r *orderRepo) GetByID(ctx context.Context, id string) (*domain.Order, error) {
	query := "SELECT id, user_id, amount, status, items, created_at, updated_at, cancelled_at FROM orders WHERE id = $1"

	var o domain.Order
	var itemsJSON []byte
	var cancelledAt sql.NullTime

	err := r.db.QueryRow(ctx, query, id).Scan(
		&o.ID,
		&o.UserID,
		&o.Amount,
		&o.Status,
		&itemsJSON,
		&o.CreatedAt,
		&o.UpdatedAt,
		&cancelledAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		r.logg.Error("failed to get order by id", "error", err, "order_id", id)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	// Deserialize items from JSON
	if err := json.Unmarshal(itemsJSON, &o.Items); err != nil {
		r.logg.Error("failed to unmarshal order items", "error", err, "order_id", id)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	if cancelledAt.Valid {
		o.CancelledAt = &cancelledAt.Time
	}

	return &o, nil
}

// GetByUserID fetches orders for a specific user with pagination
// Responsibility: Query database and translate errors to domain errors
func (r *orderRepo) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Order, error) {
	query := "SELECT id, user_id, amount, status, items, created_at, updated_at, cancelled_at FROM orders WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3"

	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		r.logg.Error("failed to get orders by user id", "error", err, "user_id", userID)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}
	defer rows.Close()

	return r.scanOrders(rows)
}

// Create inserts a new order
// Responsibility: Execute INSERT and handle database constraints
func (r *orderRepo) Create(ctx context.Context, order *domain.Order) error {
	query := "INSERT INTO orders (id, user_id, amount, status, items, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)"

	// Serialize items to JSON
	itemsJSON, err := json.Marshal(order.Items)
	if err != nil {
		r.logg.Error("failed to marshal order items", "error", err, "order_id", order.ID)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	_, err = r.db.Exec(ctx, query,
		order.ID,
		order.UserID,
		order.Amount,
		order.Status,
		itemsJSON,
		order.CreatedAt,
		order.UpdatedAt,
	)

	if err != nil {
		r.logg.Error("failed to create order", "error", err, "order_id", order.ID)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	return nil
}

// Update updates an existing order
// Responsibility: Execute UPDATE and handle database errors
func (r *orderRepo) Update(ctx context.Context, order *domain.Order) error {
	query := "UPDATE orders SET amount = $2, status = $3, items = $4, updated_at = $5, cancelled_at = $6 WHERE id = $1"

	// Serialize items to JSON
	itemsJSON, err := json.Marshal(order.Items)
	if err != nil {
		r.logg.Error("failed to marshal order items", "error", err, "order_id", order.ID)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	result, err := r.db.Exec(ctx, query,
		order.ID,
		order.Amount,
		order.Status,
		itemsJSON,
		order.UpdatedAt,
		order.CancelledAt,
	)

	if err != nil {
		r.logg.Error("failed to update order", "error", err, "order_id", order.ID)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	// Check if any rows were affected
	if result.RowsAffected() == 0 {
		return domain.ErrOrderNotFound
	}

	return nil
}

// Delete removes an order by ID
// Responsibility: Execute DELETE and handle database errors
func (r *orderRepo) Delete(ctx context.Context, id string) error {
	query := "DELETE FROM orders WHERE id = $1"

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		r.logg.Error("failed to delete order", "error", err, "order_id", id)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	// Check if any rows were affected
	if result.RowsAffected() == 0 {
		return domain.ErrOrderNotFound
	}

	return nil
}

// List retrieves a paginated list of orders
// Responsibility: Query database with pagination
func (r *orderRepo) List(ctx context.Context, limit, offset int) ([]*domain.Order, error) {
	query := "SELECT id, user_id, amount, status, items, created_at, updated_at, cancelled_at FROM orders ORDER BY created_at DESC LIMIT $1 OFFSET $2"

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		r.logg.Error("failed to list orders", "error", err)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}
	defer rows.Close()

	return r.scanOrders(rows)
}

// scanOrders is a helper method to scan multiple order rows
// Responsibility: Convert database rows to domain entities
func (r *orderRepo) scanOrders(rows pgx.Rows) ([]*domain.Order, error) {
	var orders []*domain.Order

	for rows.Next() {
		var o domain.Order
		var itemsJSON []byte
		var cancelledAt sql.NullTime

		err := rows.Scan(
			&o.ID,
			&o.UserID,
			&o.Amount,
			&o.Status,
			&itemsJSON,
			&o.CreatedAt,
			&o.UpdatedAt,
			&cancelledAt,
		)
		if err != nil {
			r.logg.Error("failed to scan order row", "error", err)
			return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
		}

		// Deserialize items from JSON
		if err := json.Unmarshal(itemsJSON, &o.Items); err != nil {
			r.logg.Error("failed to unmarshal order items", "error", err, "order_id", o.ID)
			return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
		}

		if cancelledAt.Valid {
			o.CancelledAt = &cancelledAt.Time
		}

		orders = append(orders, &o)
	}

	if err := rows.Err(); err != nil {
		r.logg.Error("error iterating order rows", "error", err)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	return orders, nil
}
