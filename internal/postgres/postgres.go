// Package postgres stores the data of the application in PostgreSQL.
package postgres

import (
	"context"
	"embed"
	"fmt"

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
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("choose the database dialect: %w", err)
	}

	// goose speaks database/sql, so the pool needs a wrapper.
	database := stdlib.OpenDBFromPool(pool)
	defer func() { _ = database.Close() }()

	if err := goose.UpContext(ctx, database, "migrations"); err != nil {
		return fmt.Errorf("apply the migrations: %w", err)
	}

	return nil
}
