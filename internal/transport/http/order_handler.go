package http

import (
	"net/http"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/TopThisHat/stdlib-golang-api/internal/usecase"
)

// OrderHandler handles HTTP requests for order operations
// Transport layer - handles HTTP concerns only, delegates business logic to service
type OrderHandler struct {
	orderService *usecase.OrderService
	logg         *logger.Logger
}

// NewOrderHandler creates a new order handler
func NewOrderHandler(orderService *usecase.OrderService, logg *logger.Logger) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
		logg:         logg,
	}
}

// CreateOrderRequest represents the request body for creating an order
type CreateOrderRequest struct {
	UserID string             `json:"user_id"`
	Items  []OrderItemRequest `json:"items"`
}

// OrderItemRequest represents an order item in the request
type OrderItemRequest struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

// OrderResponse represents the response body for order operations
type OrderResponse struct {
	ID          string              `json:"id"`
	UserID      string              `json:"user_id"`
	Amount      float64             `json:"amount"`
	Status      string              `json:"status"`
	Items       []OrderItemResponse `json:"items"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	CancelledAt *string             `json:"cancelled_at,omitempty"`
}

// OrderItemResponse represents an order item in the response
type OrderItemResponse struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}

// toOrderResponse converts a domain order to a response DTO
func toOrderResponse(o *domain.Order) *OrderResponse {
	items := make([]OrderItemResponse, len(o.Items))
	for i, item := range o.Items {
		items[i] = OrderItemResponse{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
	}

	resp := &OrderResponse{
		ID:        o.ID,
		UserID:    o.UserID,
		Amount:    o.Amount,
		Status:    string(o.Status),
		Items:     items,
		CreatedAt: o.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: o.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if o.CancelledAt != nil {
		cancelledAt := o.CancelledAt.Format("2006-01-02T15:04:05Z")
		resp.CancelledAt = &cancelledAt
	}

	return resp
}

// toOrderListResponse converts a slice of domain orders to response DTOs
func toOrderListResponse(orders []*domain.Order) []*OrderResponse {
	result := make([]*OrderResponse, len(orders))
	for i, o := range orders {
		result[i] = toOrderResponse(o)
	}
	return result
}

// toDomainOrderItems converts request items to domain order items
func toDomainOrderItems(items []OrderItemRequest) []domain.OrderItem {
	result := make([]domain.OrderItem, len(items))
	for i, item := range items {
		result[i] = domain.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
	}
	return result
}

// Create handles POST /api/orders
func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Validate required fields
	if req.UserID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "User ID is required")
		return
	}

	if len(req.Items) == 0 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "At least one item is required")
		return
	}

	// Validate items
	for i, item := range req.Items {
		if item.ProductID == "" {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Product ID is required for all items")
			return
		}
		if item.Quantity <= 0 {
			h.logg.Warn("invalid item quantity", "index", i, "quantity", item.Quantity)
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Quantity must be positive")
			return
		}
		if item.Price < 0 {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Price cannot be negative")
			return
		}
	}

	order, err := h.orderService.CreateOrder(r.Context(), req.UserID, toDomainOrderItems(req.Items))
	if err != nil {
		h.logg.Error("failed to create order", "error", err, "user_id", req.UserID)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, toOrderResponse(order))
}

// GetByID handles GET /api/orders/{id}
func (h *OrderHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Order ID is required")
		return
	}

	order, err := h.orderService.GetOrderByID(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toOrderResponse(order))
}

// GetByUserID handles GET /api/users/{user_id}/orders
func (h *OrderHandler) GetByUserID(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("user_id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "User ID is required")
		return
	}

	limit := parseIntQueryParam(r, "limit", 20)
	offset := parseIntQueryParam(r, "offset", 0)

	orders, err := h.orderService.GetOrdersByUserID(r.Context(), userID, limit, offset)
	if err != nil {
		h.logg.Error("failed to get orders by user", "error", err, "user_id", userID)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"orders": toOrderListResponse(orders),
		"limit":  limit,
		"offset": offset,
	})
}

// List handles GET /api/orders
func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQueryParam(r, "limit", 20)
	offset := parseIntQueryParam(r, "offset", 0)

	orders, err := h.orderService.ListOrders(r.Context(), limit, offset)
	if err != nil {
		h.logg.Error("failed to list orders", "error", err)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"orders": toOrderListResponse(orders),
		"limit":  limit,
		"offset": offset,
	})
}

// Confirm handles POST /api/orders/{id}/confirm
func (h *OrderHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Order ID is required")
		return
	}

	order, err := h.orderService.ConfirmOrder(r.Context(), id)
	if err != nil {
		h.logg.Error("failed to confirm order", "error", err, "order_id", id)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toOrderResponse(order))
}

// Ship handles POST /api/orders/{id}/ship
func (h *OrderHandler) Ship(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Order ID is required")
		return
	}

	order, err := h.orderService.ShipOrder(r.Context(), id)
	if err != nil {
		h.logg.Error("failed to ship order", "error", err, "order_id", id)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toOrderResponse(order))
}

// Deliver handles POST /api/orders/{id}/deliver
func (h *OrderHandler) Deliver(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Order ID is required")
		return
	}

	order, err := h.orderService.DeliverOrder(r.Context(), id)
	if err != nil {
		h.logg.Error("failed to deliver order", "error", err, "order_id", id)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toOrderResponse(order))
}

// Cancel handles POST /api/orders/{id}/cancel
func (h *OrderHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Order ID is required")
		return
	}

	order, err := h.orderService.CancelOrder(r.Context(), id)
	if err != nil {
		h.logg.Error("failed to cancel order", "error", err, "order_id", id)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toOrderResponse(order))
}
