// Package seed writes the users that a development database starts with.
//
// The application gives no visitor a way to create a user yet, so this is the
// only way rows arrive.
package seed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// Store is the part of the user store that the seed needs.
type Store interface {
	GetByEmail(ctx context.Context, email string) (user.User, error)
	Create(ctx context.Context, fullName, email, phoneNumber string) (user.User, error)
	SetPhoneNumber(ctx context.Context, id uuid.UUID, phoneNumber string) (user.User, error)
}

// Person is one entry of the seed.
type Person struct {
	FullName    string
	Email       string
	PhoneNumber string
}

// People are the users that every development database holds. The two share
// one phone number, because one person reads both messages.
var People = []Person{
	{FullName: "Arne Jørgensen", Email: "arne@ejbygruppe.dk", PhoneNumber: "+4521650113"},
	{FullName: "Test Jørgensen", Email: "test-jorgensen@ejbygruppe.dk", PhoneNumber: "+4521650113"},
}

// Users writes the missing users and leaves the others alone, so a second run
// changes nothing.
func Users(ctx context.Context, store Store) error {
	for _, person := range People {
		found, err := store.GetByEmail(ctx, person.Email)
		if err == nil {
			// The phone number arrived later than the two users, so the seed
			// fills it in.
			if found.PhoneNumber != person.PhoneNumber {
				if _, err := store.SetPhoneNumber(ctx, found.ID, person.PhoneNumber); err != nil {
					return fmt.Errorf("set the number of %s: %w", person.Email, err)
				}

				slog.Info("wrote the phone number", "email", person.Email)
			}

			slog.Info("the user is already there", "email", person.Email)

			continue
		}

		if !errors.Is(err, user.ErrNotFound) {
			return fmt.Errorf("look for %s: %w", person.Email, err)
		}

		created, err := store.Create(ctx, person.FullName, person.Email, person.PhoneNumber)
		if err != nil {
			return fmt.Errorf("create %s: %w", person.Email, err)
		}

		slog.Info("wrote the user", "email", created.Email, "id", created.ID)
	}

	return nil
}
