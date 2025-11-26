package domain

import (
	"context"
	"time"
)

// OrderStatus represents the state of an order
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// Order represents an order in the system
// This is a pure domain entity with no infrastructure concerns
type Order struct {
	ID          string
	UserID      string
	Amount      float64
	Status      OrderStatus
	Items       []OrderItem
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CancelledAt *time.Time
}

// OrderItem represents a single item in an order
type OrderItem struct {
	ProductID string
	Quantity  int
	Price     float64
}

// OrderRepository defines the contract for order persistence
// The domain defines the interface, infrastructure implements it
type OrderRepository interface {
	GetByID(ctx context.Context, id string) (*Order, error)
	GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*Order, error)
	Create(ctx context.Context, order *Order) error
	Update(ctx context.Context, order *Order) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, limit, offset int) ([]*Order, error)
}

// OrderCache defines the contract for order caching
// The domain defines the interface, infrastructure implements it
type OrderCache interface {
	Get(ctx context.Context, orderID string) (*Order, error)
	Set(ctx context.Context, order *Order) error
	Invalidate(ctx context.Context, orderID string) error
	InvalidateByUserID(ctx context.Context, userID string) error
	// Index methods for maintaining user-to-orders mapping
	AddUserOrderIndex(ctx context.Context, userID, orderID string) error
	RemoveUserOrderIndex(ctx context.Context, userID, orderID string) error
}

// NewOrder creates a new order with validation
// Business rule: Order must have valid user, positive amount, and at least one item
func NewOrder(id, userID string, items []OrderItem) (*Order, error) {
	if userID == "" {
		return nil, ErrInvalidInput
	}

	if len(items) == 0 {
		return nil, ErrInvalidInput
	}

	// Calculate total amount from items
	var amount float64
	for _, item := range items {
		if item.Quantity <= 0 {
			return nil, ErrInvalidInput
		}
		if item.Price < 0 {
			return nil, ErrInvalidOrderAmount
		}
		amount += item.Price * float64(item.Quantity)
	}

	o := &Order{
		ID:        id,
		UserID:    userID,
		Amount:    amount,
		Status:    OrderStatusPending,
		Items:     items,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return o, nil
}

// Validate ensures the order entity is in a valid state
// This is domain business logic - not persistence logic
func (o *Order) Validate() error {
	if o.ID == "" {
		return ErrInvalidInput
	}

	if o.UserID == "" {
		return ErrInvalidInput
	}

	if o.Amount < 0 {
		return ErrInvalidOrderAmount
	}

	if !o.IsValidStatus() {
		return ErrInvalidOrderStatus
	}

	if len(o.Items) == 0 {
		return ErrInvalidInput
	}

	return nil
}

// IsValidStatus checks if the current status is valid
func (o *Order) IsValidStatus() bool {
	switch o.Status {
	case OrderStatusPending, OrderStatusConfirmed, OrderStatusShipped,
		OrderStatusDelivered, OrderStatusCancelled:
		return true
	default:
		return false
	}
}

// Confirm transitions the order to confirmed status
// Business rule: Only pending orders can be confirmed
func (o *Order) Confirm() error {
	if o.Status != OrderStatusPending {
		return ErrInvalidOrderStatus
	}
	o.Status = OrderStatusConfirmed
	o.UpdatedAt = time.Now().UTC()
	return nil
}

// Ship transitions the order to shipped status
// Business rule: Only confirmed orders can be shipped
func (o *Order) Ship() error {
	if o.Status != OrderStatusConfirmed {
		return ErrInvalidOrderStatus
	}
	o.Status = OrderStatusShipped
	o.UpdatedAt = time.Now().UTC()
	return nil
}

// Deliver transitions the order to delivered status
// Business rule: Only shipped orders can be delivered
func (o *Order) Deliver() error {
	if o.Status != OrderStatusShipped {
		return ErrInvalidOrderStatus
	}
	o.Status = OrderStatusDelivered
	o.UpdatedAt = time.Now().UTC()
	return nil
}

// Cancel transitions the order to cancelled status
// Business rule: Only pending or confirmed orders can be cancelled
func (o *Order) Cancel() error {
	if o.Status != OrderStatusPending && o.Status != OrderStatusConfirmed {
		return ErrOrderCannotBeCancelled
	}
	o.Status = OrderStatusCancelled
	now := time.Now().UTC()
	o.CancelledAt = &now
	o.UpdatedAt = now
	return nil
}

// IsCancellable returns whether the order can be cancelled
func (o *Order) IsCancellable() bool {
	return o.Status == OrderStatusPending || o.Status == OrderStatusConfirmed
}

// CanBeShipped returns whether the order can be shipped
func (o *Order) CanBeShipped() bool {
	return o.Status == OrderStatusConfirmed
}

// RecalculateAmount recalculates the total amount from items
// Business rule: Amount must match sum of all items
func (o *Order) RecalculateAmount() {
	var amount float64
	for _, item := range o.Items {
		amount += item.Price * float64(item.Quantity)
	}
	o.Amount = amount
	o.UpdatedAt = time.Now().UTC()
}
