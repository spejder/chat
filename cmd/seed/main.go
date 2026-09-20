// Command seed writes the users that a development database starts with. A
// second run changes nothing.
//
// Usage: go run ./cmd/seed [-database-url ...]
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"

	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "address of the PostgreSQL server")
	flag.Parse()

	if *databaseURL == "" {
		return errors.New("no database address, set DATABASE_URL or pass -database-url")
	}

	ctx := context.Background()

	pool, err := postgres.Open(ctx, *databaseURL)
	if err != nil {
		return err
	}

	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}

	return seed.Users(ctx, postgres.NewUserStore(pool))
}
