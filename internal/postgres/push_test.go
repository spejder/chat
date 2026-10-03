package postgres_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/push"
)

// pushStores builds the stores that a push test needs.
type pushStores struct {
	push  *postgres.PushStore
	auth  *postgres.AuthStore
	users *postgres.UserStore
}

func newPushStores(t *testing.T) pushStores {
	t.Helper()

	pool := postgrestest.New(t)

	return pushStores{
		push:  postgres.NewPushStore(pool),
		auth:  postgres.NewAuthStore(pool),
		users: postgres.NewUserStore(pool),
	}
}

// session writes a session for a person and returns its key.
func session(t *testing.T, stores pushStores, userID uuid.UUID, expiresAt time.Time) []byte {
	t.Helper()

	key := []byte(uuid.NewV7().String())

	if err := stores.auth.SaveSession(t.Context(), key, userID, expiresAt); err != nil {
		t.Fatalf("save the session: %v", err)
	}

	return key
}

// subscription builds a browser with a given endpoint.
func subscription(endpoint string) push.Subscription {
	var out push.Subscription

	out.Endpoint = endpoint
	out.Keys.P256dh = "p256dh-" + endpoint
	out.Keys.Auth = "auth-" + endpoint

	return out
}

// TestTheKeyPairStaysTheSame makes sure that a second pair never replaces the
// first, because a new pair breaks every subscription.
func TestTheKeyPairStaysTheSame(t *testing.T) {
	t.Parallel()

	stores := newPushStores(t)

	if _, ok, err := stores.push.Keys(t.Context()); err != nil || ok {
		t.Fatalf("a new database holds keys: ok = %v, error = %v", ok, err)
	}

	first := push.Keys{Public: "public-1", Private: "private-1"}
	if err := stores.push.AddKeys(t.Context(), first); err != nil {
		t.Fatalf("add the first pair: %v", err)
	}

	if err := stores.push.AddKeys(t.Context(), push.Keys{Public: "public-2", Private: "private-2"}); err != nil {
		t.Fatalf("add the second pair: %v", err)
	}

	got, ok, err := stores.push.Keys(t.Context())
	if err != nil || !ok {
		t.Fatalf("read the pair: ok = %v, error = %v", ok, err)
	}

	if got != first {
		t.Errorf("the pair is %+v, want the first %+v", got, first)
	}
}

// TestASubscriptionFollowsItsSession covers the life of a browser: it moves to
// the person who signs in, it hears nothing once its session ran out, and it
// goes with its session.
func TestASubscriptionFollowsItsSession(t *testing.T) {
	t.Parallel()

	stores := newPushStores(t)
	ada, grace := twoPeople(t, stores.users)

	adaSession := session(t, stores, ada.ID, time.Now().Add(time.Hour))
	graceSession := session(t, stores, grace.ID, time.Now().Add(time.Hour))
	oldSession := session(t, stores, grace.ID, time.Now().Add(-time.Hour))

	browser := subscription("https://push.example.com/shared")

	if err := stores.push.Save(t.Context(), ada.ID, adaSession, browser); err != nil {
		t.Fatalf("save for the first person: %v", err)
	}

	// The same browser signs in as somebody else.
	if err := stores.push.Save(t.Context(), grace.ID, graceSession, browser); err != nil {
		t.Fatalf("save for the second person: %v", err)
	}

	if err := stores.push.Save(t.Context(), grace.ID, oldSession, subscription("https://push.example.com/old")); err != nil {
		t.Fatalf("save on an old session: %v", err)
	}

	targets, err := stores.push.Targets(t.Context(), []uuid.UUID{ada.ID, grace.ID})
	if err != nil {
		t.Fatalf("targets: %v", err)
	}

	if len(targets) != 1 || targets[0].UserID != grace.ID || targets[0].Subscription.Endpoint != browser.Endpoint {
		t.Fatalf("the targets are %+v, want the shared browser for the second person only", targets)
	}

	// Signing out removes the browser.
	if err := stores.auth.DeleteSession(t.Context(), graceSession); err != nil {
		t.Fatalf("delete the session: %v", err)
	}

	targets, err = stores.push.Targets(t.Context(), []uuid.UUID{grace.ID})
	if err != nil {
		t.Fatalf("targets: %v", err)
	}

	if len(targets) != 0 {
		t.Errorf("the targets are %+v after the sign out, want none", targets)
	}
}

// TestRemoveTouchesOnlyTheOwner makes sure that a person can only remove their
// own browser, and that Forget removes any.
func TestRemoveTouchesOnlyTheOwner(t *testing.T) {
	t.Parallel()

	stores := newPushStores(t)
	ada, grace := twoPeople(t, stores.users)

	adaSession := session(t, stores, ada.ID, time.Now().Add(time.Hour))
	browser := subscription("https://push.example.com/ada")

	if err := stores.push.Save(t.Context(), ada.ID, adaSession, browser); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := stores.push.Remove(t.Context(), grace.ID, browser.Endpoint); err != nil {
		t.Fatalf("remove as the wrong person: %v", err)
	}

	if targets, err := stores.push.Targets(t.Context(), []uuid.UUID{ada.ID}); err != nil || len(targets) != 1 {
		t.Fatalf("the wrong person removed the browser: %+v, %v", targets, err)
	}

	if err := stores.push.Forget(t.Context(), browser.Endpoint); err != nil {
		t.Fatalf("forget: %v", err)
	}

	if targets, err := stores.push.Targets(t.Context(), []uuid.UUID{ada.ID}); err != nil || len(targets) != 0 {
		t.Errorf("the browser is still there after forget: %+v, %v", targets, err)
	}
}

// TestUnreadCountsEveryConversation makes sure that the count for the badge
// adds up the unread messages of every conversation of a person, and drops
// after a read.
func TestUnreadCountsEveryConversation(t *testing.T) {
	t.Parallel()

	pool := postgrestest.New(t)
	store := postgres.NewPushStore(pool)
	chats := postgres.NewChatStore(pool)
	ada, grace := twoPeople(t, postgres.NewUserStore(pool))

	first, err := chats.Create(t.Context(), "Lunch", ada.ID, []uuid.UUID{ada.ID, grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("create the first conversation: %v", err)
	}

	if _, err := chats.Create(t.Context(), "Party", ada.ID, []uuid.UUID{ada.ID, grace.ID}, "Who brings cake?"); err != nil {
		t.Fatalf("create the second conversation: %v", err)
	}

	if _, err := chats.AddMessage(t.Context(), first.ID, ada.ID, "Twelve o'clock"); err != nil {
		t.Fatalf("add a message: %v", err)
	}

	counts, err := store.Unread(t.Context(), []uuid.UUID{ada.ID, grace.ID})
	if err != nil {
		t.Fatalf("unread: %v", err)
	}

	// Ada wrote everything, so only Grace has something to read.
	if counts[grace.ID] != 3 || counts[ada.ID] != 0 {
		t.Errorf("the counts are %v, want 3 for Grace and 0 for Ada", counts)
	}

	if _, _, err := chats.MarkRead(t.Context(), first.ID, grace.ID); err != nil {
		t.Fatalf("mark as read: %v", err)
	}

	counts, err = store.Unread(t.Context(), []uuid.UUID{grace.ID})
	if err != nil {
		t.Fatalf("unread: %v", err)
	}

	if counts[grace.ID] != 1 {
		t.Errorf("the count after a read is %d, want 1", counts[grace.ID])
	}
}
