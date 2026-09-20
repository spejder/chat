package seed_test

import (
	"testing"

	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/seed"
)

// TestUsersRunsTwice makes sure that the seed writes the people once and that
// a second run changes nothing.
func TestUsersRunsTwice(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(postgrestest.New(t))

	for run := 1; run <= 2; run++ {
		if err := seed.Users(t.Context(), store); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}

		users, err := store.List(t.Context())
		if err != nil {
			t.Fatalf("run %d, list: %v", run, err)
		}

		if len(users) != len(seed.People) {
			t.Fatalf("run %d: the table holds %d users, want %d", run, len(users), len(seed.People))
		}
	}

	for _, person := range seed.People {
		found, err := store.GetByEmail(t.Context(), person.Email)
		if err != nil {
			t.Fatalf("get %s: %v", person.Email, err)
		}

		if found.FullName != person.FullName {
			t.Errorf("the user %s is %q, want %q", person.Email, found.FullName, person.FullName)
		}
	}
}
