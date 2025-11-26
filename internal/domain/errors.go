package domain

import "errors"

// Domain errors - sentinel errors that can be compared with errors.Is()
var (
	// User errors
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInvalidUserEmail  = errors.New("invalid user email")
	ErrInvalidUserID     = errors.New("invalid user id")

	// Order errors
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderAlreadyExists     = errors.New("order already exists")
	ErrInvalidOrderStatus     = errors.New("invalid order status")
	ErrInvalidOrderAmount     = errors.New("invalid order amount")
	ErrOrderCannotBeCancelled = errors.New("order cannot be cancelled")

	// Generic errors
	ErrInvalidInput  = errors.New("invalid input")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrInternalError = errors.New("internal error")
	ErrDatabaseError = errors.New("database error")
	ErrConflict      = errors.New("resource conflict")

	// Cache errors
	ErrCacheMiss = errors.New("cache miss")

	// Blob storage errors
	ErrBlobNotFound       = errors.New("blob not found")
	ErrBlobAlreadyExists  = errors.New("blob already exists")
	ErrBlobUploadFailed   = errors.New("blob upload failed")
	ErrBlobDownloadFailed = errors.New("blob download failed")
	ErrBlobDeleteFailed   = errors.New("blob delete failed")
	ErrInvalidBlobKey     = errors.New("invalid blob key")
)
