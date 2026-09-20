package postgres_test

import (
	"testing"
	"uuid"

	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/user"
)

// newChatStore builds the two stores that a conversation test needs.
func newChatStore(t *testing.T) (*postgres.ChatStore, *postgres.UserStore) {
	t.Helper()

	pool := postgrestest.New(t)

	return postgres.NewChatStore(pool), postgres.NewUserStore(pool)
}

// twoPeople writes two users for a test.
func twoPeople(t *testing.T, users *postgres.UserStore) (user.User, user.User) {
	t.Helper()

	first, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4500000001")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	second, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4500000002")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	return first, second
}

// TestCreateAndRead writes a conversation with a first message and reads it
// back from both sides.
func TestCreateAndReadAConversation(t *testing.T) {
	t.Parallel()

	store, users := newChatStore(t)
	ada, grace := twoPeople(t, users)

	conversation, err := store.Create(t.Context(), "Lunch", ada.ID, []uuid.UUID{ada.ID, grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if conversation.Subject != "Lunch" {
		t.Errorf("subject = %q, want %q", conversation.Subject, "Lunch")
	}

	for _, person := range []user.User{ada, grace} {
		member, err := store.IsParticipant(t.Context(), conversation.ID, person.ID)
		if err != nil || !member {
			t.Errorf("%s takes part = %v, error = %v", person.FullName, member, err)
		}
	}

	messages, err := store.Messages(t.Context(), conversation.ID)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}

	if len(messages) != 1 || messages[0].Body != "Are you in?" {
		t.Fatalf("the messages are %+v, want the first message", messages)
	}

	if messages[0].AuthorName != ada.FullName {
		t.Errorf("the author is %q, want %q", messages[0].AuthorName, ada.FullName)
	}

	people, err := store.Participants(t.Context(), conversation.ID)
	if err != nil {
		t.Fatalf("participants: %v", err)
	}

	if len(people) != 2 {
		t.Errorf("the conversation holds %d people, want 2", len(people))
	}
}

// TestUnreadCount covers the count on the list page before and after a read.
func TestUnreadCount(t *testing.T) {
	t.Parallel()

	store, users := newChatStore(t)
	ada, grace := twoPeople(t, users)

	conversation, err := store.Create(t.Context(), "Lunch", ada.ID, []uuid.UUID{ada.ID, grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// The writer has nothing to read, the other person has one message.
	if unread := unreadFor(t, store, ada, conversation.ID); unread != 0 {
		t.Errorf("the writer has %d unread, want 0", unread)
	}

	if unread := unreadFor(t, store, grace, conversation.ID); unread != 1 {
		t.Errorf("the reader has %d unread, want 1", unread)
	}

	if err := store.MarkRead(t.Context(), conversation.ID, grace.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	if unread := unreadFor(t, store, grace, conversation.ID); unread != 0 {
		t.Errorf("after the read there are %d unread, want 0", unread)
	}

	if _, err := store.AddMessage(t.Context(), conversation.ID, ada.ID, "Twelve o'clock?"); err != nil {
		t.Fatalf("add a message: %v", err)
	}

	if unread := unreadFor(t, store, grace, conversation.ID); unread != 1 {
		t.Errorf("after the new message there are %d unread, want 1", unread)
	}
}

// TestListNamesTheOthers makes sure that the line of the list names the other
// people and holds the time of the newest message.
func TestListNamesTheOthers(t *testing.T) {
	t.Parallel()

	store, users := newChatStore(t)
	ada, grace := twoPeople(t, users)

	conversation, err := store.Create(t.Context(), "Lunch", ada.ID, []uuid.UUID{ada.ID, grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	summaries, err := store.List(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("the list holds %d lines, want 1", len(summaries))
	}

	if summaries[0].Others != grace.FullName {
		t.Errorf("the others are %q, want %q", summaries[0].Others, grace.FullName)
	}

	if summaries[0].LastMessageAt.Before(conversation.CreatedAt) {
		t.Error("the time of the newest message is older than the conversation")
	}
}

// TestAStrangerSeesNothing makes sure that a conversation stays with its
// people.
func TestAStrangerSeesNothing(t *testing.T) {
	t.Parallel()

	store, users := newChatStore(t)
	ada, grace := twoPeople(t, users)

	stranger, err := users.Create(t.Context(), "Alan Turing", "alan@example.com", "+4500000003")
	if err != nil {
		t.Fatalf("create the stranger: %v", err)
	}

	conversation, err := store.Create(t.Context(), "Lunch", ada.ID, []uuid.UUID{ada.ID, grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	member, err := store.IsParticipant(t.Context(), conversation.ID, stranger.ID)
	if err != nil {
		t.Fatalf("is participant: %v", err)
	}

	if member {
		t.Error("the stranger takes part in the conversation")
	}

	summaries, err := store.List(t.Context(), stranger.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("the stranger sees %d conversations, want 0", len(summaries))
	}
}

// unreadFor reads the unread count of one person for one conversation.
func unreadFor(t *testing.T, store *postgres.ChatStore, person user.User, id uuid.UUID) int {
	t.Helper()

	summaries, err := store.List(t.Context(), person.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	for _, summary := range summaries {
		if summary.ID == id {
			return summary.Unread
		}
	}

	t.Fatalf("%s does not see the conversation", person.FullName)

	return 0
}
