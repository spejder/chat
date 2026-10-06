package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/postgres/db"
	"github.com/spejder/chat/internal/remind"
)

// RemindStore finds the people who missed a message. It carries out the
// Store interface of internal/remind.
type RemindStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewRemindStore builds a store on top of a pool of connections.
func NewRemindStore(pool *pgxpool.Pool) *RemindStore {
	return &RemindStore{pool: pool, queries: db.New(pool)}
}

// Claim finds the due pairs of person and conversation and notes the SMS for
// them. Only one server instance claims at a time: the others find the lock
// taken and claim nothing, so nobody gets two SMS.
func (s *RemindStore) Claim(ctx context.Context, dueBefore, notBefore time.Time) ([]remind.Due, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("start the transaction: %w", err)
	}

	// A commit makes this rollback a no-op.
	defer func() { _ = tx.Rollback(ctx) }()

	queries := s.queries.WithTx(tx)

	locked, err := queries.LockReminders(ctx)
	if err != nil {
		return nil, fmt.Errorf("take the lock: %w", err)
	}

	if !locked {
		return nil, nil
	}

	rows, err := queries.ClaimReminders(ctx, db.ClaimRemindersParams{
		NotBefore: notBefore,
		DueBefore: dueBefore,
	})
	if err != nil {
		return nil, fmt.Errorf("claim the reminders: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("finish the transaction: %w", err)
	}

	due := make([]remind.Due, 0, len(rows))
	for _, row := range rows {
		due = append(due, remind.Due{
			UserID:         row.UserID,
			ConversationID: row.ConversationID,
			PhoneNumber:    row.PhoneNumber,
			Subject:        row.Subject,
			Writers:        row.Writers,
		})
	}

	return due, nil
}
