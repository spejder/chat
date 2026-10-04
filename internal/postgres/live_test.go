package postgres_test

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/live"
	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
)

// TestAChangeTravelsThroughPostgres sends a change with NOTIFY and waits for
// it to arrive at a hub through LISTEN, the way it travels between two
// server instances.
func TestAChangeTravelsThroughPostgres(t *testing.T) {
	t.Parallel()

	pool := postgrestest.New(t)
	hub := live.New()

	ctx, cancel := context.WithCancel(t.Context())

	stopped := make(chan struct{})

	go func() {
		postgres.Listen(ctx, pool, hub)
		close(stopped)
	}()

	t.Cleanup(func() {
		cancel()
		<-stopped
	})

	ada := uuid.NewV7()
	conversation := uuid.NewV7()

	events, unsubscribe := hub.Subscribe(ada)
	defer unsubscribe()

	broadcaster := postgres.NewBroadcaster(pool)

	// The listener needs a moment before its LISTEN stands, so the change
	// goes out again until it arrives.
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)

	defer tick.Stop()

	for {
		select {
		case got := <-events:
			if got.Conversation != conversation || got.Kind != live.KindChanged {
				t.Fatalf("the hub received %+v, want a change of %v", got, conversation)
			}

			return
		case <-tick.C:
			broadcaster.Changed(t.Context(), conversation, []uuid.UUID{ada})
		case <-deadline:
			t.Fatal("the change never arrived")
		}
	}
}
