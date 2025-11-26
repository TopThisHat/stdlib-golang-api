package http

import (
	"net/http"
	"strings"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/TopThisHat/stdlib-golang-api/internal/usecase"
)

// UserHandler handles HTTP requests for user operations
// Transport layer - handles HTTP concerns only, delegates business logic to service
type UserHandler struct {
	userService *usecase.UserService
	logg        *logger.Logger
}

// NewUserHandler creates a new user handler
func NewUserHandler(userService *usecase.UserService, logg *logger.Logger) *UserHandler {
	return &UserHandler{
		userService: userService,
		logg:        logg,
	}
}

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UpdateUserRequest represents the request body for updating a user
type UpdateUserRequest struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// UserResponse represents the response body for user operations
type UserResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// toUserResponse converts a domain user to a response DTO
func toUserResponse(u *domain.User) *UserResponse {
	return &UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: u.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// toUserListResponse converts a slice of domain users to response DTOs
func toUserListResponse(users []*domain.User) []*UserResponse {
	result := make([]*UserResponse, len(users))
	for i, u := range users {
		result[i] = toUserResponse(u)
	}
	return result
}

// Create handles POST /api/users
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.Name) == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Name is required")
		return
	}

	if strings.TrimSpace(req.Email) == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Email is required")
		return
	}

	user, err := h.userService.CreateUser(r.Context(), req.Name, req.Email)
	if err != nil {
		h.logg.Error("failed to create user", "error", err)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, toUserResponse(user))
}

// GetByID handles GET /api/users/{id}
func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "User ID is required")
		return
	}

	user, err := h.userService.GetUserByID(r.Context(), id)
	if err != nil {
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toUserResponse(user))
}

// Update handles PUT /api/users/{id}
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "User ID is required")
		return
	}

	var req UpdateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	user, err := h.userService.UpdateUser(r.Context(), id, req.Name, req.Email)
	if err != nil {
		h.logg.Error("failed to update user", "error", err, "user_id", id)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, toUserResponse(user))
}

// Delete handles DELETE /api/users/{id}
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "User ID is required")
		return
	}

	if err := h.userService.DeleteUser(r.Context(), id); err != nil {
		h.logg.Error("failed to delete user", "error", err, "user_id", id)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

// List handles GET /api/users
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQueryParam(r, "limit", 20)
	offset := parseIntQueryParam(r, "offset", 0)

	users, err := h.userService.ListUsers(r.Context(), limit, offset)
	if err != nil {
		h.logg.Error("failed to list users", "error", err)
		handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"users":  toUserListResponse(users),
		"limit":  limit,
		"offset": offset,
	})
}
