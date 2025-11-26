package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/google/uuid"
)

// OrderService orchestrates order-related business operations
// This layer contains business logic and coordinates between domain and repository
type OrderService struct {
	orderRepo  domain.OrderRepository
	userRepo   domain.UserRepository
	orderCache domain.OrderCache
	logg       *logger.Logger
}

// NewOrderService creates a new order service
func NewOrderService(orderRepo domain.OrderRepository, userRepo domain.UserRepository, orderCache domain.OrderCache, logg *logger.Logger) *OrderService {
	return &OrderService{
		orderRepo:  orderRepo,
		userRepo:   userRepo,
		orderCache: orderCache,
		logg:       logg,
	}
}

// CreateOrder creates a new order with validation
// Business logic: Validates user exists, validates order items, generates ID
func (s *OrderService) CreateOrder(ctx context.Context, userID string, items []domain.OrderItem) (*domain.Order, error) {
	// Business rule: Verify user exists before creating order
	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if err == domain.ErrUserNotFound {
			s.logg.Warn("cannot create order for non-existent user", "user_id", userID)
			return nil, err
		}
		s.logg.Error("failed to verify user", "error", err, "user_id", userID)
		return nil, fmt.Errorf("%w: failed to verify user", domain.ErrInternalError)
	}

	// Generate unique ID for the order
	id := uuid.New().String()

	// Create domain entity (includes validation and amount calculation)
	order, err := domain.NewOrder(id, userID, items)
	if err != nil {
		s.logg.Warn("invalid order data", "error", err, "user_id", userID)
		return nil, err
	}

	// Persist the order
	if err := s.orderRepo.Create(ctx, order); err != nil {
		s.logg.Error("failed to create order", "error", err, "order_id", order.ID)
		return nil, err
	}

	// Cache the new order and add to user index
	if s.orderCache != nil {
		if err := s.orderCache.Set(ctx, order); err != nil {
			s.logg.Warn("cache set failed", "error", err, "order_id", order.ID)
		}
		if err := s.orderCache.AddUserOrderIndex(ctx, userID, order.ID); err != nil {
			s.logg.Warn("cache user index add failed", "error", err, "order_id", order.ID)
		}
	}

	s.logg.Info("order created successfully", "order_id", order.ID, "user_id", userID, "amount", order.Amount)
	return order, nil
}

// GetOrderByID retrieves an order by ID
// Uses cache-aside pattern: check cache first, then database
func (s *OrderService) GetOrderByID(ctx context.Context, id string) (*domain.Order, error) {
	if id == "" {
		return nil, domain.ErrInvalidInput
	}

	// Try cache first
	if s.orderCache != nil {
		if order, err := s.orderCache.Get(ctx, id); err == nil {
			return order, nil
		} else if !errors.Is(err, domain.ErrCacheMiss) {
			s.logg.Warn("cache get failed", "error", err, "order_id", id)
		}
	}

	// Cache miss or no cache, fetch from repository
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Populate cache for future requests
	if s.orderCache != nil {
		if err := s.orderCache.Set(ctx, order); err != nil {
			s.logg.Warn("cache set failed", "error", err, "order_id", id)
		}
	}

	return order, nil
}

// GetOrdersByUserID retrieves orders for a specific user
func (s *OrderService) GetOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]*domain.Order, error) {
	if userID == "" {
		return nil, domain.ErrInvalidInput
	}

	// Business rule: Set reasonable pagination limits
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if offset < 0 {
		offset = 0
	}

	orders, err := s.orderRepo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		s.logg.Error("failed to get orders by user id", "error", err, "user_id", userID)
		return nil, err
	}

	return orders, nil
}

// ConfirmOrder confirms a pending order
// Business logic: Uses domain method to enforce status transition rules
func (s *OrderService) ConfirmOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Domain enforces business rules for state transitions
	if err := order.Confirm(); err != nil {
		s.logg.Warn("cannot confirm order", "error", err, "order_id", id, "status", order.Status)
		return nil, err
	}

	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logg.Error("failed to update order", "error", err, "order_id", id)
		return nil, err
	}

	// Invalidate cache after status change
	if s.orderCache != nil {
		if err := s.orderCache.Invalidate(ctx, id); err != nil {
			s.logg.Warn("cache invalidate failed", "error", err, "order_id", id)
		}
	}

	s.logg.Info("order confirmed", "order_id", id)
	return order, nil
}

// ShipOrder marks an order as shipped
// Business logic: Uses domain method to enforce status transition rules
func (s *OrderService) ShipOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Domain enforces business rules for state transitions
	if err := order.Ship(); err != nil {
		s.logg.Warn("cannot ship order", "error", err, "order_id", id, "status", order.Status)
		return nil, err
	}

	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logg.Error("failed to update order", "error", err, "order_id", id)
		return nil, err
	}

	// Invalidate cache after status change
	if s.orderCache != nil {
		if err := s.orderCache.Invalidate(ctx, id); err != nil {
			s.logg.Warn("cache invalidate failed", "error", err, "order_id", id)
		}
	}

	s.logg.Info("order shipped", "order_id", id)
	return order, nil
}

// DeliverOrder marks an order as delivered
// Business logic: Uses domain method to enforce status transition rules
func (s *OrderService) DeliverOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Domain enforces business rules for state transitions
	if err := order.Deliver(); err != nil {
		s.logg.Warn("cannot deliver order", "error", err, "order_id", id, "status", order.Status)
		return nil, err
	}

	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logg.Error("failed to update order", "error", err, "order_id", id)
		return nil, err
	}

	// Invalidate cache after status change
	if s.orderCache != nil {
		if err := s.orderCache.Invalidate(ctx, id); err != nil {
			s.logg.Warn("cache invalidate failed", "error", err, "order_id", id)
		}
	}

	s.logg.Info("order delivered", "order_id", id)
	return order, nil
}

// CancelOrder cancels an order
// Business logic: Uses domain method to enforce cancellation rules
func (s *OrderService) CancelOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Domain enforces business rules for cancellation
	if err := order.Cancel(); err != nil {
		s.logg.Warn("cannot cancel order", "error", err, "order_id", id, "status", order.Status)
		return nil, err
	}

	// Business logic: Could add refund processing here
	// e.g., s.paymentService.ProcessRefund(ctx, order)

	if err := s.orderRepo.Update(ctx, order); err != nil {
		s.logg.Error("failed to update order", "error", err, "order_id", id)
		return nil, err
	}

	// Invalidate cache after cancellation
	if s.orderCache != nil {
		if err := s.orderCache.Invalidate(ctx, id); err != nil {
			s.logg.Warn("cache invalidate failed", "error", err, "order_id", id)
		}
		if err := s.orderCache.RemoveUserOrderIndex(ctx, order.UserID, id); err != nil {
			s.logg.Warn("cache user index remove failed", "error", err, "order_id", id)
		}
	}

	s.logg.Info("order cancelled", "order_id", id)
	return order, nil
}

// ListOrders retrieves a paginated list of all orders
func (s *OrderService) ListOrders(ctx context.Context, limit, offset int) ([]*domain.Order, error) {
	// Business rule: Set reasonable pagination limits
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if offset < 0 {
		offset = 0
	}

	orders, err := s.orderRepo.List(ctx, limit, offset)
	if err != nil {
		s.logg.Error("failed to list orders", "error", err)
		return nil, err
	}

	return orders, nil
}
