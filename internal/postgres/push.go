package postgres

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/postgres/db"
	"github.com/spejder/chat/internal/push"
)

// PushStore keeps the key pair of the site and the browsers that subscribed.
// It carries out the Store interface of internal/push.
type PushStore struct {
	queries *db.Queries
}

// NewPushStore builds a store on top of a pool of connections.
func NewPushStore(pool *pgxpool.Pool) *PushStore {
	return &PushStore{queries: db.New(pool)}
}

// Keys reads the key pair of the site.
func (s *PushStore) Keys(ctx context.Context) (push.Keys, bool, error) {
	row, err := s.queries.GetVAPIDKeys(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return push.Keys{}, false, nil
	}

	if err != nil {
		return push.Keys{}, false, fmt.Errorf("read the push keys: %w", err)
	}

	return push.Keys{Public: row.PublicKey, Private: row.PrivateKey}, true, nil
}

// AddKeys stores a key pair unless the site has one already.
func (s *PushStore) AddKeys(ctx context.Context, keys push.Keys) error {
	if err := s.queries.InsertVAPIDKeys(ctx, db.InsertVAPIDKeysParams{
		PublicKey:  keys.Public,
		PrivateKey: keys.Private,
	}); err != nil {
		return fmt.Errorf("store the push keys: %w", err)
	}

	return nil
}

// Save stores a browser for a person and a session.
func (s *PushStore) Save(ctx context.Context, userID uuid.UUID, sessionKey []byte, subscription push.Subscription) error {
	if err := s.queries.UpsertSubscription(ctx, db.UpsertSubscriptionParams{
		ID:         uuid.NewV7(),
		UserID:     userID,
		SessionKey: sessionKey,
		Endpoint:   subscription.Endpoint,
		P256dh:     subscription.Keys.P256dh,
		Auth:       subscription.Keys.Auth,
	}); err != nil {
		return fmt.Errorf("store the subscription: %w", err)
	}

	return nil
}

// Remove deletes one browser of a person.
func (s *PushStore) Remove(ctx context.Context, userID uuid.UUID, endpoint string) error {
	if err := s.queries.DeleteSubscription(ctx, db.DeleteSubscriptionParams{
		Endpoint: endpoint,
		UserID:   userID,
	}); err != nil {
		return fmt.Errorf("remove the subscription: %w", err)
	}

	return nil
}

// Forget deletes a browser that its push service no longer knows.
func (s *PushStore) Forget(ctx context.Context, endpoint string) error {
	if err := s.queries.DeleteSubscriptionByEndpoint(ctx, endpoint); err != nil {
		return fmt.Errorf("forget the subscription: %w", err)
	}

	return nil
}

// Targets reads the browsers of some people whose session still runs.
func (s *PushStore) Targets(ctx context.Context, userIDs []uuid.UUID) ([]push.Target, error) {
	rows, err := s.queries.ListSubscriptionsForUsers(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("list the subscriptions: %w", err)
	}

	targets := make([]push.Target, 0, len(rows))

	for _, row := range rows {
		target := push.Target{UserID: row.UserID}
		target.Subscription.Endpoint = row.Endpoint
		target.Subscription.Keys.P256dh = row.P256dh
		target.Subscription.Keys.Auth = row.Auth

		targets = append(targets, target)
	}

	return targets, nil
}
