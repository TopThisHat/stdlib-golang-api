package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
)

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError represents an error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// respondJSON sends a JSON response with the given status code
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := APIResponse{
		Success: status >= 200 && status < 300,
		Data:    data,
	}

	json.NewEncoder(w).Encode(response)
}

// respondError sends an error response with the given status code
func respondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
	}

	json.NewEncoder(w).Encode(response)
}

// mapDomainErrorToHTTP maps domain errors to appropriate HTTP status codes
func mapDomainErrorToHTTP(err error) (int, string, string) {
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		return http.StatusNotFound, "USER_NOT_FOUND", "User not found"
	case errors.Is(err, domain.ErrOrderNotFound):
		return http.StatusNotFound, "ORDER_NOT_FOUND", "Order not found"
	case errors.Is(err, domain.ErrUserAlreadyExists):
		return http.StatusConflict, "USER_ALREADY_EXISTS", "User already exists"
	case errors.Is(err, domain.ErrOrderAlreadyExists):
		return http.StatusConflict, "ORDER_ALREADY_EXISTS", "Order already exists"
	case errors.Is(err, domain.ErrInvalidUserEmail):
		return http.StatusBadRequest, "INVALID_EMAIL", "Invalid email format"
	case errors.Is(err, domain.ErrInvalidUserID):
		return http.StatusBadRequest, "INVALID_USER_ID", "Invalid user ID"
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "INVALID_INPUT", "Invalid input data"
	case errors.Is(err, domain.ErrInvalidOrderStatus):
		return http.StatusBadRequest, "INVALID_ORDER_STATUS", "Invalid order status transition"
	case errors.Is(err, domain.ErrInvalidOrderAmount):
		return http.StatusBadRequest, "INVALID_ORDER_AMOUNT", "Invalid order amount"
	case errors.Is(err, domain.ErrOrderCannotBeCancelled):
		return http.StatusBadRequest, "ORDER_CANNOT_BE_CANCELLED", "Order cannot be cancelled in current state"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized access"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "FORBIDDEN", "Access forbidden"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "CONFLICT", "Resource conflict"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred"
	}
}

// handleError handles domain errors and sends appropriate HTTP responses
func handleError(w http.ResponseWriter, err error) {
	status, code, message := mapDomainErrorToHTTP(err)
	respondError(w, status, code, message)
}

// parseIntQueryParam parses an integer query parameter with a default value
func parseIntQueryParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}

	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}

	return parsed
}

// decodeJSON decodes JSON from request body into the target struct
func decodeJSON(r *http.Request, target interface{}) error {
	if r.Body == nil {
		return domain.ErrInvalidInput
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return domain.ErrInvalidInput
	}

	return nil
}
