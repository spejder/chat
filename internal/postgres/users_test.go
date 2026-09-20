package postgres_test

import (
	"errors"
	"testing"
	"uuid"

	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/user"
)

// TestCreateAndRead writes a user and reads it back, which also proves that
// the identifier and the times survive the round trip.
func TestCreateAndRead(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(postgrestest.New(t))

	created, err := store.Create(t.Context(), "Ada Lovelace", "ada@example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if created.ID == uuid.Nil() {
		t.Error("the user has no identifier")
	}

	// The version sits in the high half of the seventh byte.
	if version := created.ID[6] >> 4; version != 7 {
		t.Errorf("the identifier is version %d, want 7", version)
	}

	if created.CreatedAt.IsZero() {
		t.Error("the user has no creation time")
	}

	read, err := store.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if read != created {
		t.Errorf("read %+v, want %+v", read, created)
	}
}

// TestGetByEmailIgnoresCase covers the lookup by address.
func TestGetByEmailIgnoresCase(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(postgrestest.New(t))

	created, err := store.Create(t.Context(), "Grace Hopper", "grace@example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	read, err := store.GetByEmail(t.Context(), "GRACE@Example.COM")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}

	if read.ID != created.ID {
		t.Errorf("read the user %s, want %s", read.ID, created.ID)
	}
}

// TestUnknownUser covers both lookups when the store holds nothing.
func TestUnknownUser(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(postgrestest.New(t))

	if _, err := store.Get(t.Context(), uuid.NewV7()); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("get: error = %v, want %v", err, user.ErrNotFound)
	}

	if _, err := store.GetByEmail(t.Context(), "nobody@example.com"); !errors.Is(err, user.ErrNotFound) {
		t.Errorf("get by email: error = %v, want %v", err, user.ErrNotFound)
	}
}

// TestDuplicateEmail makes sure that one address cannot arrive twice, not
// even in another case.
func TestDuplicateEmail(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(postgrestest.New(t))

	if _, err := store.Create(t.Context(), "Alan Turing", "alan@example.com"); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := store.Create(t.Context(), "Alan T", "ALAN@example.com")
	if !errors.Is(err, user.ErrDuplicateEmail) {
		t.Errorf("error = %v, want %v", err, user.ErrDuplicateEmail)
	}
}

// TestListIsInCreationOrder makes sure that the order of the list follows the
// identifier, which version 7 builds from the time.
func TestListIsInCreationOrder(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(postgrestest.New(t))

	names := []string{"First", "Second", "Third"}
	for i, name := range names {
		if _, err := store.Create(t.Context(), name, name+"@example.com"); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	users, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(users) != len(names) {
		t.Fatalf("the list holds %d users, want %d", len(users), len(names))
	}

	for i, name := range names {
		if users[i].FullName != name {
			t.Errorf("user %d is %q, want %q", i, users[i].FullName, name)
		}
	}
}
