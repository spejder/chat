package postgres

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/postgres/db"
	"github.com/spejder/chat/internal/user"
)

// uniqueViolation is the code that Postgres returns when a value breaks a
// unique index.
const uniqueViolation = "23505"

// UserStore reads and writes users.
type UserStore struct {
	queries *db.Queries
}

// NewUserStore builds a store on top of a pool of connections.
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{queries: db.New(pool)}
}

// Create writes a new user. The store makes the identifier, a UUID version 7.
//
// The application gives no visitor a way to create a user. The seed and the
// tests use this method.
func (s *UserStore) Create(ctx context.Context, fullName, email string) (user.User, error) {
	row, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		ID:       uuid.NewV7(),
		FullName: fullName,
		Email:    email,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return user.User{}, user.ErrDuplicateEmail
		}

		return user.User{}, fmt.Errorf("create the user: %w", err)
	}

	return toUser(row), nil
}

// Get reads one user by identifier.
func (s *UserStore) Get(ctx context.Context, id uuid.UUID) (user.User, error) {
	row, err := s.queries.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user.User{}, user.ErrNotFound
		}

		return user.User{}, fmt.Errorf("read the user: %w", err)
	}

	return toUser(row), nil
}

// GetByEmail reads one user by email address. The case does not matter.
func (s *UserStore) GetByEmail(ctx context.Context, email string) (user.User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user.User{}, user.ErrNotFound
		}

		return user.User{}, fmt.Errorf("read the user by email: %w", err)
	}

	return toUser(row), nil
}

// List reads every user, oldest first.
func (s *UserStore) List(ctx context.Context) ([]user.User, error) {
	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the users: %w", err)
	}

	users := make([]user.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, toUser(row))
	}

	return users, nil
}

// toUser turns a database row into the type that the rest of the application
// uses.
func toUser(row db.User) user.User {
	return user.User{
		ID:        row.ID,
		FullName:  row.FullName,
		Email:     row.Email,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
