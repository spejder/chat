// Package postgrestest gives every test its own database.
//
// The tests need a running server. `task db:up` starts one, and DATABASE_URL
// says where it listens.
package postgrestest

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/postgres"
)

// New creates a database, applies the migrations, and returns a pool for it.
// The database goes away when the test ends.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	address := os.Getenv("DATABASE_URL")
	if address == "" {
		t.Fatal("DATABASE_URL is empty. Start the database with 'task db:up'.")
	}

	admin, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatalf("reach the database server: %v", err)
	}

	name := "chat_test_" + strings.ToLower(rand.Text()[:10])

	// The name comes from this file, not from a request, and a database name
	// cannot be a parameter.
	if _, err := admin.Exec(t.Context(), `CREATE DATABASE "`+name+`"`); err != nil {
		admin.Close()
		t.Fatalf("create the test database: %v", err)
	}

	pool, err := postgres.Open(t.Context(), withDatabase(t, address, name))
	if err != nil {
		admin.Close()
		t.Fatalf("open the test database: %v", err)
	}

	if err := postgres.Migrate(t.Context(), pool); err != nil {
		pool.Close()
		admin.Close()
		t.Fatalf("migrate the test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		// The context of the test is already cancelled here.
		if _, err := admin.Exec(context.Background(), `DROP DATABASE "`+name+`" WITH (FORCE)`); err != nil {
			t.Errorf("drop the test database %s: %v", name, err)
		}

		admin.Close()
	})

	return pool
}

// withDatabase returns the same address with another database name.
func withDatabase(t *testing.T, address, name string) string {
	t.Helper()

	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatalf("read DATABASE_URL: %v", err)
	}

	parsed.Path = "/" + name

	return parsed.String()
}
