// Package user holds the user of the chat and the errors that a store of
// users can return. The package knows nothing about SQL.
package user

import (
	"errors"
	"time"
	"uuid"
)

// User is a person who can take part in a chat.
type User struct {
	// ID is a UUID version 7. Such an identifier starts with the time of
	// creation, so a list in identifier order is a list in creation order.
	ID        uuid.UUID
	FullName  string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	// ErrNotFound says that the store holds no such user.
	ErrNotFound = errors.New("user not found")

	// ErrDuplicateEmail says that another user already has that email
	// address. The store compares addresses without case.
	ErrDuplicateEmail = errors.New("a user with that email address exists")
)
