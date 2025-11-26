package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/google/uuid"
)

// UserCache defines the caching interface for users
// Defined in usecase layer to avoid dependency on infrastructure
type UserCache interface {
	Get(ctx context.Context, userID string) (*domain.User, error)
	Set(ctx context.Context, user *domain.User) error
	Invalidate(ctx context.Context, userID string) error
}

// UserService orchestrates user-related business operations
// This layer contains business logic and coordinates between domain and repository
type UserService struct {
	userRepo  domain.UserRepository
	userCache UserCache
	logg      *logger.Logger
}

// NewUserService creates a new user service
func NewUserService(userRepo domain.UserRepository, userCache UserCache, logg *logger.Logger) *UserService {
	return &UserService{
		userRepo:  userRepo,
		userCache: userCache,
		logg:      logg,
	}
}

// CreateUser creates a new user with validation
// Business logic: Validates user data, ensures unique email, generates ID
func (s *UserService) CreateUser(ctx context.Context, name, email string) (*domain.User, error) {
	// Generate unique ID for the user
	id := uuid.New().String()

	// Create domain entity (includes validation)
	user, err := domain.NewUser(id, name, email)
	if err != nil {
		s.logg.Warn("invalid user data", "error", err, "email", email)
		return nil, err
	}

	// Business rule: Check if email already exists
	existingUser, err := s.userRepo.GetByEmail(ctx, user.NormalizeEmail())
	if err != nil && err != domain.ErrUserNotFound {
		s.logg.Error("failed to check existing user", "error", err, "email", email)
		return nil, fmt.Errorf("%w: failed to validate user uniqueness", domain.ErrInternalError)
	}

	if existingUser != nil {
		s.logg.Warn("user already exists", "email", email)
		return nil, domain.ErrUserAlreadyExists
	}

	// Persist the user
	if err := s.userRepo.Create(ctx, user); err != nil {
		s.logg.Error("failed to create user", "error", err, "user_id", user.ID)
		return nil, err
	}

	s.logg.Info("user created successfully", "user_id", user.ID, "email", email)
	return user, nil
}

// GetUserByID retrieves a user by ID
// Uses cache-aside pattern: check cache first, then database
func (s *UserService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	if id == "" {
		return nil, domain.ErrInvalidUserID
	}

	// Try cache first
	if s.userCache != nil {
		if user, err := s.userCache.Get(ctx, id); err == nil {
			return user, nil
		} else if !errors.Is(err, domain.ErrCacheMiss) {
			s.logg.Warn("cache get failed", "error", err, "user_id", id)
		}
	}

	// Cache miss or no cache, fetch from repository
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Populate cache for future requests
	if s.userCache != nil {
		if err := s.userCache.Set(ctx, user); err != nil {
			s.logg.Warn("cache set failed", "error", err, "user_id", id)
		}
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	if email == "" {
		return nil, domain.ErrInvalidUserEmail
	}

	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// UpdateUser updates a user's information
// Business logic: Validates changes, ensures email uniqueness if changed
func (s *UserService) UpdateUser(ctx context.Context, id, name, email string) (*domain.User, error) {
	// Retrieve existing user
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update name if provided
	if name != "" && name != user.Name {
		if err := user.UpdateName(name); err != nil {
			s.logg.Warn("invalid name update", "error", err, "user_id", id)
			return nil, err
		}
	}

	// Update email if provided and different
	if email != "" && email != user.Email {
		// Business rule: Check if new email already exists
		existingUser, err := s.userRepo.GetByEmail(ctx, email)
		if err != nil && err != domain.ErrUserNotFound {
			s.logg.Error("failed to check existing email", "error", err, "email", email)
			return nil, fmt.Errorf("%w: failed to validate email uniqueness", domain.ErrInternalError)
		}

		if existingUser != nil && existingUser.ID != id {
			s.logg.Warn("email already in use", "email", email)
			return nil, domain.ErrUserAlreadyExists
		}

		if err := user.UpdateEmail(email); err != nil {
			s.logg.Warn("invalid email update", "error", err, "user_id", id)
			return nil, err
		}
	}

	// Persist changes
	if err := s.userRepo.Update(ctx, user); err != nil {
		s.logg.Error("failed to update user", "error", err, "user_id", id)
		return nil, err
	}

	// Invalidate cache after successful update
	if s.userCache != nil {
		if err := s.userCache.Invalidate(ctx, id); err != nil {
			s.logg.Warn("cache invalidate failed", "error", err, "user_id", id)
		}
	}

	s.logg.Info("user updated successfully", "user_id", id)
	return user, nil
}

// DeleteUser removes a user
// Business logic: Could add additional checks (e.g., prevent deletion if user has active orders)
func (s *UserService) DeleteUser(ctx context.Context, id string) error {
	if id == "" {
		return domain.ErrInvalidUserID
	}

	// Verify user exists
	_, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Business rule: Add any pre-deletion checks here
	// For example: check if user has active orders, subscriptions, etc.

	if err := s.userRepo.Delete(ctx, id); err != nil {
		s.logg.Error("failed to delete user", "error", err, "user_id", id)
		return err
	}

	// Invalidate cache after successful deletion
	if s.userCache != nil {
		if err := s.userCache.Invalidate(ctx, id); err != nil {
			s.logg.Warn("cache invalidate failed", "error", err, "user_id", id)
		}
	}

	s.logg.Info("user deleted successfully", "user_id", id)
	return nil
}

// ListUsers retrieves a paginated list of users
func (s *UserService) ListUsers(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	// Business rule: Set reasonable pagination limits
	if limit <= 0 || limit > 100 {
		limit = 20 // Default limit
	}

	if offset < 0 {
		offset = 0
	}

	users, err := s.userRepo.List(ctx, limit, offset)
	if err != nil {
		s.logg.Error("failed to list users", "error", err)
		return nil, err
	}

	return users, nil
}
