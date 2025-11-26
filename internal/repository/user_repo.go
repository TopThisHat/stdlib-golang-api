package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TopThisHat/stdlib-golang-api/internal/domain"
	"github.com/TopThisHat/stdlib-golang-api/internal/logger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// userRepo is the PostgreSQL implementation of domain.UserRepository
// It contains NO business logic - only data persistence
type userRepo struct {
	db   *pgxpool.Pool
	logg *logger.Logger
}

// NewUserRepo creates a Postgres-backed user repository
func NewUserRepo(db *pgxpool.Pool, logg *logger.Logger) domain.UserRepository {
	return &userRepo{db: db, logg: logg}
}

// GetByID fetches a user by ID
// Responsibility: Query database and translate errors to domain errors
func (r *userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	query := "SELECT id, name, email, created_at, updated_at FROM users WHERE id = $1"

	var u domain.User
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Name,
		&u.Email,
		&u.CreatedAt,
		&u.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		r.logg.Error("failed to get user by id", "error", err, "user_id", id)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	return &u, nil
}

// GetByEmail fetches a user by email address
// Responsibility: Query database and translate errors to domain errors
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := "SELECT id, name, email, created_at, updated_at FROM users WHERE LOWER(email) = LOWER($1)"

	var u domain.User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&u.ID,
		&u.Name,
		&u.Email,
		&u.CreatedAt,
		&u.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		r.logg.Error("failed to get user by email", "error", err, "email", email)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	return &u, nil
}

// Create inserts a new user
// Responsibility: Execute INSERT and handle database constraints
func (r *userRepo) Create(ctx context.Context, user *domain.User) error {
	query := "INSERT INTO users (id, name, email, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)"

	_, err := r.db.Exec(ctx, query,
		user.ID,
		user.Name,
		user.Email,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		// Translate database-specific errors to domain errors
		if pgErr, ok := err.(*pgconn.PgError); ok {
			// 23505 is Postgres unique violation
			if pgErr.Code == "23505" {
				if strings.Contains(pgErr.ConstraintName, "email") {
					return domain.ErrUserAlreadyExists
				}
			}
		}
		r.logg.Error("failed to create user", "error", err, "user_id", user.ID)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	return nil
}

// Update updates an existing user
// Responsibility: Execute UPDATE and handle database errors
func (r *userRepo) Update(ctx context.Context, user *domain.User) error {
	query := "UPDATE users SET name = $2, email = $3, updated_at = $4 WHERE id = $1"

	result, err := r.db.Exec(ctx, query,
		user.ID,
		user.Name,
		user.Email,
		user.UpdatedAt,
	)

	if err != nil {
		// Check for unique constraint violations
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				if strings.Contains(pgErr.ConstraintName, "email") {
					return domain.ErrUserAlreadyExists
				}
			}
		}
		r.logg.Error("failed to update user", "error", err, "user_id", user.ID)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	// Check if any rows were affected
	if result.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// Delete removes a user by ID
// Responsibility: Execute DELETE and handle database errors
func (r *userRepo) Delete(ctx context.Context, id string) error {
	query := "DELETE FROM users WHERE id = $1"

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		r.logg.Error("failed to delete user", "error", err, "user_id", id)
		return fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	// Check if any rows were affected
	if result.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// List retrieves a paginated list of users
// Responsibility: Query database with pagination
func (r *userRepo) List(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	query := "SELECT id, name, email, created_at, updated_at FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2"

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		r.logg.Error("failed to list users", "error", err)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var u domain.User
		err := rows.Scan(
			&u.ID,
			&u.Name,
			&u.Email,
			&u.CreatedAt,
			&u.UpdatedAt,
		)
		if err != nil {
			r.logg.Error("failed to scan user row", "error", err)
			return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
		}
		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		r.logg.Error("error iterating user rows", "error", err)
		return nil, fmt.Errorf("%w: %v", domain.ErrDatabaseError, err)
	}

	return users, nil
}
