// Package auth signs people in and out.
//
// A visitor types an email address. A person with a passkey signs in with it.
// A person without one receives a six digit code by SMS and can create a
// passkey right after.
package auth

import (
	"context"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// Credential is one stored passkey. Data holds the credential of the WebAuthn
// library as JSON, because that library owns the shape.
type Credential struct {
	ID   []byte
	Data []byte
}

// Code is a sign in code that went out by SMS. The store keeps the hash, not
// the code.
type Code struct {
	ID       uuid.UUID
	Hash     []byte
	Attempts int
}

// Challenge is the state between the two halves of a passkey ceremony.
type Challenge struct {
	UserID uuid.UUID
	Data   []byte
}

// Purpose says which ceremony a challenge belongs to.
const (
	PurposeLogin    = "login"
	PurposeRegister = "register"
)

// Users reads people. The store in internal/postgres fits this interface.
type Users interface {
	Get(ctx context.Context, id uuid.UUID) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
}

// Store keeps everything that a sign in needs. A lookup that finds nothing
// returns false, not an error.
type Store interface {
	Credentials(ctx context.Context, userID uuid.UUID) ([]Credential, error)
	AddCredential(ctx context.Context, userID uuid.UUID, id, data []byte) error
	UpdateCredential(ctx context.Context, id, data []byte) error

	SaveChallenge(ctx context.Context, id, userID uuid.UUID, purpose string, data []byte, expiresAt time.Time) error
	TakeChallenge(ctx context.Context, id uuid.UUID, purpose string) (Challenge, bool, error)

	ConsumeCodes(ctx context.Context, userID uuid.UUID) error
	SaveCode(ctx context.Context, id, userID uuid.UUID, hash []byte, expiresAt time.Time) error
	LatestCode(ctx context.Context, userID uuid.UUID) (Code, bool, error)
	CountCodeAttempt(ctx context.Context, id uuid.UUID) (int, error)
	ConsumeCode(ctx context.Context, id uuid.UUID) error

	SaveSession(ctx context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error
	SessionUser(ctx context.Context, hash []byte) (user.User, bool, error)
	DeleteSession(ctx context.Context, hash []byte) error
}
