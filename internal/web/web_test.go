package web_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
	"github.com/spejder/chat/internal/web"
)

// render turns a component into markup for a reader who is signed in.
func render(t *testing.T, component templ.Component, children templ.Component) string {
	t.Helper()

	ctx := auth.WithUser(context.Background(), user.User{
		ID:       uuid.NewV7(),
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
	})

	if children != nil {
		ctx = templ.WithChildren(ctx, children)
	}

	var out strings.Builder

	if err := component.Render(ctx, &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	return out.String()
}

// TestTheShellHoldsThePersonAndTheList makes sure that every page of a signed
// in person carries the sidebar.
func TestTheShellHoldsThePersonAndTheList(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.September, 20, 13, 45, 7, 0, time.UTC)
	conversation := chat.Conversation{ID: uuid.NewV7(), Subject: "Lunch", CreatedAt: at}

	summaries := []chat.Summary{{
		Conversation:  conversation,
		Others:        "Grace Hopper",
		LastMessageAt: at,
		Unread:        2,
	}}

	body := render(t, web.Shell(summaries, conversation.ID, "Lunch", true, "abc123"), web.Conversations())

	for _, want := range []string{
		"Ada Lovelace",
		"ada@example.com",
		"Sign out",
		"Lunch",
		"Grace Hopper",
		"2 new",
		"Start a conversation",
		`data-hx-get="/conversations/list?v=abc123&amp;current=` + conversation.ID.String() + `"`,
		`data-unread="2"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}
}

// TestTheOpenConversationIsMarked makes sure that the list shows which
// conversation the reader has open.
func TestTheOpenConversationIsMarked(t *testing.T) {
	t.Parallel()

	open := chat.Conversation{ID: uuid.NewV7(), Subject: "Lunch"}
	other := chat.Conversation{ID: uuid.NewV7(), Subject: "Holiday"}

	summaries := []chat.Summary{
		{Conversation: open, Others: "Grace Hopper"},
		{Conversation: other, Others: "Alan Turing"},
	}

	body := render(t, web.ConversationList(summaries, open.ID, "abc123"), nil)

	if !strings.Contains(body, `aria-current="page"`) {
		t.Errorf("the open conversation carries no mark: %s", body)
	}

	if strings.Count(body, `aria-current="page"`) != 1 {
		t.Error("more than one line carries the mark")
	}
}

// TestLayoutPutsTheChildrenInTheBody makes sure that a page can place its own
// markup inside the shell.
func TestLayoutPutsTheChildrenInTheBody(t *testing.T) {
	t.Parallel()

	body := render(t, web.Layout("Title", "Description"), templ.Raw("<span>marker</span>"))

	if !strings.Contains(body, "<title>Title</title>") {
		t.Errorf("the shell does not hold the title: %s", body)
	}

	_, after, found := strings.Cut(body, "<body")
	if !found || !strings.Contains(after, "marker") {
		t.Errorf("the children are not inside the body: %s", body)
	}
}

// TestTheSidesOfAConversation makes sure that the reader sits on one side and
// everybody else on the other, and that a long word stays inside its bubble.
func TestTheSidesOfAConversation(t *testing.T) {
	t.Parallel()

	reader := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace"}
	other := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper"}

	at := time.Now()

	messages := []chat.Message{
		{
			ID:         uuid.NewV7(),
			AuthorID:   other.ID,
			AuthorName: other.FullName,
			Body:       strings.Repeat("a", 200),
			CreatedAt:  at,
		},
		{
			ID:        uuid.NewV7(),
			AuthorID:  reader.ID,
			Body:      "Mine",
			CreatedAt: at.Add(time.Minute),
		},
	}

	var out strings.Builder
	if err := web.Messages(messages, web.Panel{Reader: reader, People: 3}).Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	body := out.String()

	for _, want := range []string{"justify-start", "justify-end", "break-words", other.FullName, "title="} {
		if !strings.Contains(body, want) {
			t.Errorf("the markup misses %q", want)
		}
	}

	// The reader needs no name, because the side says who wrote it.
	if strings.Contains(body, reader.FullName) {
		t.Error("the markup names the reader")
	}
}
