package live_test

import (
	"testing"
	"uuid"

	"github.com/spejder/chat/internal/live"
)

// receive reads one event, or reports false when none waits.
func receive(events <-chan live.Event) (uuid.UUID, bool) {
	select {
	case event, open := <-events:
		return event.Conversation, open
	default:
		return uuid.Nil(), false
	}
}

// TestAnEventReachesThePagesOfItsPeople covers who hears an event.
func TestAnEventReachesThePagesOfItsPeople(t *testing.T) {
	t.Parallel()

	hub := live.New()
	ada, grace, alan := uuid.NewV7(), uuid.NewV7(), uuid.NewV7()

	adaPhone, cancelPhone := hub.Subscribe(ada)
	defer cancelPhone()

	adaLaptop, cancelLaptop := hub.Subscribe(ada)
	defer cancelLaptop()

	alanPage, cancelAlan := hub.Subscribe(alan)
	defer cancelAlan()

	conversation := uuid.NewV7()
	hub.Deliver(live.Event{Kind: live.KindChanged, Conversation: conversation}, []uuid.UUID{ada, grace})

	for name, events := range map[string]<-chan live.Event{"phone": adaPhone, "laptop": adaLaptop} {
		if got, ok := receive(events); !ok || got != conversation {
			t.Errorf("the %s got %v (%v), want the conversation", name, got, ok)
		}
	}

	if _, ok := receive(alanPage); ok {
		t.Error("a person outside the conversation heard the event")
	}
}

// TestCancelAndCloseEndTheStreams makes sure that a page that leaves hears
// nothing more, and that closing the hub ends every stream.
func TestCancelAndCloseEndTheStreams(t *testing.T) {
	t.Parallel()

	hub := live.New()
	ada := uuid.NewV7()

	gone, cancel := hub.Subscribe(ada)
	cancel()
	cancel()

	if _, open := <-gone; open {
		t.Error("a cancelled stream is still open")
	}

	hub.Deliver(live.Event{Kind: live.KindChanged, Conversation: uuid.NewV7()}, []uuid.UUID{ada})

	staying, cancelStaying := hub.Subscribe(ada)
	defer cancelStaying()

	hub.Close()

	if _, open := <-staying; open {
		t.Error("a stream is still open after the hub closed")
	}

	late, cancelLate := hub.Subscribe(ada)
	defer cancelLate()

	if _, open := <-late; open {
		t.Error("a stream opened after the close is still open")
	}
}

// TestASlowPageDoesNotBlock makes sure that a page that never reads cannot
// hold up the others.
func TestASlowPageDoesNotBlock(t *testing.T) {
	t.Parallel()

	hub := live.New()
	ada := uuid.NewV7()

	_, cancel := hub.Subscribe(ada)
	defer cancel()

	for range 100 {
		hub.Deliver(live.Event{Kind: live.KindChanged, Conversation: uuid.NewV7()}, []uuid.UUID{ada})
	}
}
