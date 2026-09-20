package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/postgres/db"
	"github.com/spejder/chat/internal/user"
)

// AuthStore keeps the passkeys, the codes, the started ceremonies and the
// sessions. It carries out the Store interface of internal/auth.
type AuthStore struct {
	queries *db.Queries
}

// NewAuthStore builds a store on top of a pool of connections.
func NewAuthStore(pool *pgxpool.Pool) *AuthStore {
	return &AuthStore{queries: db.New(pool)}
}

// Credentials reads the passkeys of one person.
func (s *AuthStore) Credentials(ctx context.Context, userID uuid.UUID) ([]auth.Credential, error) {
	rows, err := s.queries.ListCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list the passkeys: %w", err)
	}

	credentials := make([]auth.Credential, 0, len(rows))
	for _, row := range rows {
		credentials = append(credentials, auth.Credential{ID: row.CredentialID, Data: row.Data})
	}

	return credentials, nil
}

// AddCredential stores a new passkey.
func (s *AuthStore) AddCredential(ctx context.Context, userID uuid.UUID, id, data []byte) error {
	if err := s.queries.CreateCredential(ctx, db.CreateCredentialParams{
		CredentialID: id,
		UserID:       userID,
		Data:         data,
	}); err != nil {
		return fmt.Errorf("store the passkey: %w", err)
	}

	return nil
}

// UpdateCredential writes the passkey back after a sign in, which keeps the
// counter of the authenticator up to date.
func (s *AuthStore) UpdateCredential(ctx context.Context, id, data []byte) error {
	if err := s.queries.UpdateCredential(ctx, db.UpdateCredentialParams{
		CredentialID: id,
		Data:         data,
	}); err != nil {
		return fmt.Errorf("update the passkey: %w", err)
	}

	return nil
}

// SaveChallenge stores the state of a started ceremony.
func (s *AuthStore) SaveChallenge(ctx context.Context, id, userID uuid.UUID, purpose string, data []byte, expiresAt time.Time) error {
	if err := s.queries.DeleteExpiredChallenges(ctx); err != nil {
		return fmt.Errorf("clean up the old attempts: %w", err)
	}

	if err := s.queries.CreateChallenge(ctx, db.CreateChallengeParams{
		ID:        id,
		UserID:    userID,
		Purpose:   purpose,
		Data:      data,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("store the attempt: %w", err)
	}

	return nil
}

// TakeChallenge reads a started ceremony and removes it, so nobody can use it
// twice.
func (s *AuthStore) TakeChallenge(ctx context.Context, id uuid.UUID, purpose string) (auth.Challenge, bool, error) {
	row, err := s.queries.TakeChallenge(ctx, db.TakeChallengeParams{ID: id, Purpose: purpose})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.Challenge{}, false, nil
		}

		return auth.Challenge{}, false, fmt.Errorf("read the attempt: %w", err)
	}

	return auth.Challenge{UserID: row.UserID, Data: row.Data}, true, nil
}

// ConsumeCodes makes every code of one person useless.
func (s *AuthStore) ConsumeCodes(ctx context.Context, userID uuid.UUID) error {
	if err := s.queries.ConsumeCodes(ctx, userID); err != nil {
		return fmt.Errorf("stop the codes: %w", err)
	}

	return nil
}

// SaveCode stores the hash of a new code.
func (s *AuthStore) SaveCode(ctx context.Context, id, userID uuid.UUID, hash []byte, expiresAt time.Time) error {
	if err := s.queries.DeleteExpiredCodes(ctx); err != nil {
		return fmt.Errorf("clean up the old codes: %w", err)
	}

	if err := s.queries.CreateCode(ctx, db.CreateCodeParams{
		ID:        id,
		UserID:    userID,
		CodeHash:  hash,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("store the code: %w", err)
	}

	return nil
}

// LatestCode reads the newest code that is still alive.
func (s *AuthStore) LatestCode(ctx context.Context, userID uuid.UUID) (auth.Code, bool, error) {
	row, err := s.queries.GetLatestCode(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.Code{}, false, nil
		}

		return auth.Code{}, false, fmt.Errorf("read the code: %w", err)
	}

	return auth.Code{ID: row.ID, Hash: row.CodeHash, Attempts: int(row.Attempts)}, true, nil
}

// CountCodeAttempt counts one try and returns the new number.
func (s *AuthStore) CountCodeAttempt(ctx context.Context, id uuid.UUID) (int, error) {
	attempts, err := s.queries.CountCodeAttempt(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("count the attempt: %w", err)
	}

	return int(attempts), nil
}

// ConsumeCode makes one code useless.
func (s *AuthStore) ConsumeCode(ctx context.Context, id uuid.UUID) error {
	if err := s.queries.ConsumeCode(ctx, id); err != nil {
		return fmt.Errorf("use up the code: %w", err)
	}

	return nil
}

// SaveSession stores a signed in browser.
func (s *AuthStore) SaveSession(ctx context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error {
	if err := s.queries.DeleteExpiredSessions(ctx); err != nil {
		return fmt.Errorf("clean up the old sessions: %w", err)
	}

	if err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: hash,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}); err != nil {
		return fmt.Errorf("store the session: %w", err)
	}

	return nil
}

// SessionUser reads the person behind a live session.
func (s *AuthStore) SessionUser(ctx context.Context, hash []byte) (user.User, bool, error) {
	row, err := s.queries.GetSessionUser(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return user.User{}, false, nil
		}

		return user.User{}, false, fmt.Errorf("read the session: %w", err)
	}

	return toUser(row), true, nil
}

// DeleteSession ends one session.
func (s *AuthStore) DeleteSession(ctx context.Context, hash []byte) error {
	if err := s.queries.DeleteSession(ctx, hash); err != nil {
		return fmt.Errorf("end the session: %w", err)
	}

	return nil
}

// AuthStore must carry out the interface that internal/auth declares.
var _ auth.Store = (*AuthStore)(nil)
