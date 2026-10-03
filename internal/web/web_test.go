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

// readerID is the person who reads every page of these tests.
var readerID = uuid.NewV7()

// render turns a component into markup for a reader who is signed in.
func render(t *testing.T, component templ.Component, children templ.Component) string {
	t.Helper()

	ctx := auth.WithUser(context.Background(), user.User{
		ID:       readerID,
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

	reader := user.User{ID: readerID, FullName: "Ada Lovelace"}
	other := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper"}

	page := web.ShellPage{
		Summaries:   summaries,
		Current:     conversation.ID,
		Title:       "Lunch",
		Subject:     "Lunch",
		People:      []user.User{reader, other},
		SidebarOpen: true,
		Version:     "abc123",
	}

	body := render(t, web.Shell(page), web.Conversations())

	for _, want := range []string{
		"Ada Lovelace",
		"ada@example.com",
		"Sign out",
		"Lunch",
		`2<span class="sr-only"> unread</span>`,
		"Start a conversation",
		`aria-controls="new-conversation"`,
		`data-hx-get="/conversations/list?v=abc123&amp;current=` + conversation.ID.String() + `"`,
		`data-unread="2"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}

	// The top bar names the other person and leaves the reader out.
	_, bar, _ := strings.Cut(body, `data-slot="breadcrumb-page"`)
	bar, _, _ = strings.Cut(bar, "</")

	if !strings.Contains(bar, "Grace Hopper") || strings.Contains(bar, "Ada Lovelace") {
		t.Errorf("the top bar reads %q, want only the other person", bar)
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

// TestTheBlankRoomShowsTheMark makes sure that the list page shows the mark
// of the application, hidden from a screen reader, and that only a page that
// asks for it opens the sheet on a phone.
func TestTheBlankRoomShowsTheMark(t *testing.T) {
	t.Parallel()

	room := render(t, web.Conversations(), nil)
	if !strings.Contains(room, "data-mark") || !strings.Contains(room, `aria-hidden="true"`) {
		t.Errorf("the room holds no hidden mark: %s", room)
	}

	for _, open := range []bool{true, false} {
		body := render(t, web.Shell(web.ShellPage{Title: "Conversations", OpenOnPhone: open}), web.Conversations())

		if got := strings.Contains(body, "data-open-on-phone"); got != open {
			t.Errorf("OpenOnPhone = %v, but the page carries the mark: %v", open, got)
		}
	}
}

// TestAnEmptyListOffersTheDialog makes sure that a person without a
// conversation gets a button that opens the dialog, and that the pen carries
// a tooltip.
func TestAnEmptyListOffersTheDialog(t *testing.T) {
	t.Parallel()

	list := render(t, web.ConversationList(nil, uuid.Nil(), "abc123"), nil)
	if !strings.Contains(list, "Start a conversation") || !strings.Contains(list, `aria-controls="new-conversation"`) {
		t.Errorf("the empty list offers no way to start: %s", list)
	}

	shell := render(t, web.Shell(web.ShellPage{Title: "Conversations"}), web.Conversations())
	if !strings.Contains(shell, "data-tui-tooltip-trigger") {
		t.Error("the pen carries no tooltip")
	}
}

// TestThePageFollowsTheViewport makes sure that the shell fills the window as
// it is right now, and that a phone keyboard shrinks the page instead of
// pushing the top bar away.
func TestThePageFollowsTheViewport(t *testing.T) {
	t.Parallel()

	body := render(t, web.Shell(web.ShellPage{Title: "Conversations"}), web.Conversations())

	for _, want := range []string{"interactive-widget=resizes-content", "h-dvh"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}
}
