package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/live"
	"github.com/spejder/chat/internal/postgres/db"
	"github.com/spejder/chat/internal/user"
)

// channel is the name that NOTIFY and LISTEN share.
const channel = "chat_changed"

// relisten is the pause before the listener tries again after a broken line.
const relisten = 2 * time.Second

// change is the payload of one notification: the kind, the conversation, the
// writer of a typing event, and the people whose open pages must hear about
// it.
type change struct {
	Kind         string      `json:"kind"`
	Conversation uuid.UUID   `json:"conversation"`
	From         uuid.UUID   `json:"from,omitzero"`
	Name         string      `json:"name,omitempty"`
	People       []uuid.UUID `json:"people"`
}

// Broadcaster sends a change to every server instance through NOTIFY. It
// carries out the Broadcaster interface of internal/chat.
type Broadcaster struct {
	queries *db.Queries
}

// NewBroadcaster builds a broadcaster on top of a pool of connections.
func NewBroadcaster(pool *pgxpool.Pool) *Broadcaster {
	return &Broadcaster{queries: db.New(pool)}
}

// Changed sends the change. A failure costs only the speed of the update,
// because every page still asks now and then, so it goes to the log.
func (b *Broadcaster) Changed(ctx context.Context, conversationID uuid.UUID, people []uuid.UUID) {
	b.send(ctx, change{Kind: live.KindChanged, Conversation: conversationID, People: people})
}

// Typing sends that a person writes in the conversation right now.
func (b *Broadcaster) Typing(ctx context.Context, conversationID uuid.UUID, writer user.User, people []uuid.UUID) {
	b.send(ctx, change{
		Kind:         live.KindTyping,
		Conversation: conversationID,
		From:         writer.ID,
		Name:         writer.FullName,
		People:       people,
	})
}

// send writes one notification.
func (b *Broadcaster) send(ctx context.Context, sent change) {
	payload, err := json.Marshal(sent)
	if err != nil {
		slog.Error("could not write the change", "error", err)

		return
	}

	if err := b.queries.Notify(ctx, string(payload)); err != nil {
		slog.Error("could not send the change", "error", err)
	}
}

// Deliverer hands an event to the open pages of one server instance. The
// hub in internal/live carries it out.
type Deliverer interface {
	Deliver(event live.Event, people []uuid.UUID)
}

// Listen hands every change of every instance, this one too, to the
// deliverer. It holds one connection of the pool for as long as it runs,
// starts again after a broken line, and returns when the context ends.
func Listen(ctx context.Context, pool *pgxpool.Pool, deliverer Deliverer) {
	for {
		err := listenOnce(ctx, pool, deliverer)
		if ctx.Err() != nil {
			return
		}

		slog.Error("the line for live updates broke", "error", err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(relisten):
		}
	}
}

// listenOnce holds one connection until it breaks. The connection leaves the
// pool for good, because a connection that listens must never serve a
// query of somebody else.
func listenOnce(ctx context.Context, pool *pgxpool.Pool, deliverer Deliverer) error {
	acquired, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("take a connection: %w", err)
	}

	conn := acquired.Hijack()

	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()

		_ = conn.Close(closeCtx)
	}()

	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait for a change: %w", err)
		}

		var received change
		if err := json.Unmarshal([]byte(notification.Payload), &received); err != nil {
			slog.Error("could not read a change", "error", err)

			continue
		}

		if received.Conversation == uuid.Nil() || (received.Kind != live.KindChanged && received.Kind != live.KindTyping) {
			slog.Error("could not read a change", "error", errors.New("no conversation or an unknown kind"))

			continue
		}

		deliverer.Deliver(live.Event{
			Kind:         received.Kind,
			Conversation: received.Conversation,
			From:         received.From,
			Name:         received.Name,
		}, received.People)
	}
}
