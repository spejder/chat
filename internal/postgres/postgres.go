// Package postgres stores the data of the application in PostgreSQL.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// migrations holds the schema steps. They travel inside the binary, so a
// deployment needs no extra files.
//
//go:embed migrations/*.sql
var migrations embed.FS

// Open builds the pool of connections and asks the server whether it answers.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("read the database address: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open the database: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("reach the database: %w", err)
	}

	return pool, nil
}

// Migrate brings the schema to the newest step. It does nothing when the
// database is already there.
//
// A goose provider keeps its state to itself, so many tests can migrate their
// own databases at the same time. The package level functions of goose share
// one global state and race.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	steps, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("read the migrations: %w", err)
	}

	// goose speaks database/sql, so the pool needs a wrapper.
	database := stdlib.OpenDBFromPool(pool)

	provider, err := goose.NewProvider(goose.DialectPostgres, database, steps,
		goose.WithDisableGlobalRegistry(true),
		goose.WithLogger(goose.NopLogger()),
	)
	if err != nil {
		_ = database.Close()

		return fmt.Errorf("prepare the migrations: %w", err)
	}

	// Close closes the wrapper, not the pool behind it.
	defer func() { _ = provider.Close() }()

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply the migrations: %w", err)
	}

	return nil
}
