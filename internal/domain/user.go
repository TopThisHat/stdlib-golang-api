package domain

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// User represents a user in the system
// This is a pure domain entity with no infrastructure concerns
type User struct {
	ID        string
	Name      string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserRepository defines the contract for user persistence
// The domain defines the interface, infrastructure implements it
type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, limit, offset int) ([]*User, error)
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// NewUser creates a new user with validation
// Business rule: User must have valid email and non-empty name
func NewUser(id, name, email string) (*User, error) {
	u := &User{
		ID:        id,
		Name:      name,
		Email:     email,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := u.Validate(); err != nil {
		return nil, err
	}

	return u, nil
}

// Validate ensures the user entity is in a valid state
// This is domain business logic - not persistence logic
func (u *User) Validate() error {
	if strings.TrimSpace(u.ID) == "" {
		return ErrInvalidUserID
	}

	if strings.TrimSpace(u.Name) == "" {
		return ErrInvalidInput
	}

	if !u.IsValidEmail() {
		return ErrInvalidUserEmail
	}

	return nil
}

// IsValidEmail checks if the email format is valid
// Business rule: Email must match standard email pattern
func (u *User) IsValidEmail() bool {
	email := strings.TrimSpace(strings.ToLower(u.Email))
	if email == "" {
		return false
	}
	return emailRegex.MatchString(email)
}

// UpdateName updates the user's name with validation
// Business rule: Name cannot be empty
func (u *User) UpdateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidInput
	}
	u.Name = strings.TrimSpace(name)
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// UpdateEmail updates the user's email with validation
// Business rule: Email must be valid format
func (u *User) UpdateEmail(email string) error {
	u.Email = email
	if !u.IsValidEmail() {
		return ErrInvalidUserEmail
	}
	u.UpdatedAt = time.Now().UTC()
	return nil
}

// NormalizeEmail returns the normalized email (lowercase, trimmed)
func (u *User) NormalizeEmail() string {
	return strings.ToLower(strings.TrimSpace(u.Email))
}
