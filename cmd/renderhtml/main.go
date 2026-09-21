// Command renderhtml writes the pages and the fragments of the application
// into a directory, so an HTML validator can read them as files.
//
// Usage: go run ./cmd/renderhtml [directory]
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"uuid"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
	"github.com/spejder/chat/internal/web"
)

// fragmentShell puts a fragment into a legal document, because a validator
// reads a file as a whole document.
const fragmentShell = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Fragment</title></head>
<body>%s</body>
</html>
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "renderhtml: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dir := "tmp/html"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("make the directory: %w", err)
	}

	// The time is fixed, so two runs write the same bytes.
	at := time.Date(2026, time.September, 20, 13, 45, 7, 0, time.UTC)

	conversation := chat.Conversation{
		ID:        uuid.MustParse("01a0beac-c12a-7474-9a13-a077fb9162ad"),
		Subject:   "Lunch",
		CreatedAt: at,
	}

	people := []user.User{
		{ID: uuid.MustParse("01a0beac-c12a-7474-9a13-a077fb9162ae"), FullName: "Ada Lovelace"},
		{ID: uuid.MustParse("01a0beac-c12a-7474-9a13-a077fb9162af"), FullName: "Grace Hopper"},
	}

	// Two people across two days, so the markup holds a date line, both
	// sides and a group that runs over more than one message.
	messages := []chat.Message{
		{ID: uuid.NewV7(), AuthorID: people[0].ID, AuthorName: people[0].FullName, Body: "Are you in?", CreatedAt: at.AddDate(0, 0, -1)},
		{ID: uuid.NewV7(), AuthorID: people[1].ID, AuthorName: people[1].FullName, Body: "I am in", CreatedAt: at},
		{ID: uuid.NewV7(), AuthorID: people[1].ID, AuthorName: people[1].FullName, Body: "Twelve o'clock?", CreatedAt: at.Add(time.Minute)},
		{ID: uuid.NewV7(), AuthorID: people[0].ID, AuthorName: people[0].FullName, Body: "See you there", CreatedAt: at.Add(2 * time.Minute)},
	}

	summaries := []chat.Summary{{Conversation: conversation, Others: "Grace Hopper", LastMessageAt: at, Unread: 2}}

	// The second person has read everything, so the newest own message
	// carries its mark.
	panel := web.Panel{
		Reader: people[0],
		Since:  at.Add(-time.Hour),
		People: len(people),
		Readers: []chat.Reader{
			{ID: people[0].ID, Name: people[0].FullName, LastReadAt: at},
			{ID: people[1].ID, Name: people[1].FullName, LastReadAt: at.Add(time.Hour)},
		},
	}

	pages := map[string]templ.Component{
		"login.html":            web.Login(web.EmailPanel("", "")),
		"conversations.html":    inShell(summaries, uuid.Nil(), "All conversations", web.Conversations()),
		"conversation.html":     inShell(summaries, conversation.ID, conversation.Subject, web.ConversationPage(conversation, people, messages, panel, "4-none-0-2026-09-21", true)),
		"new-conversation.html": inShell(summaries, uuid.Nil(), "Start a conversation", web.NewConversation(people, "Lunch", "Are you in?", "")),
	}

	fragments := map[string]templ.Component{
		"conversation-list.html": web.ConversationList(summaries, conversation.ID, "abc123"),
		"messages.html":          web.Messages(messages, panel),
		"older-block.html":       web.OlderBlock(conversation, messages, web.Panel{Reader: people[0], People: 2, History: true}, true),
		"login-code.html":        web.CodePanel("ada@example.com", "That code is wrong. Try again."),
		"login-passkey.html":     web.PasskeyPanel("ada@example.com", `{"publicKey":{}}`, "01a0beac-c12a-7474-9a13-a077fb9162ad"),
		"login-offer.html":       web.PasskeyOffer(`{"publicKey":{}}`, "01a0beac-c12a-7474-9a13-a077fb9162ad"),
	}

	for name, page := range pages {
		markup, err := render(page)
		if err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}

		if err := write(filepath.Join(dir, name), markup); err != nil {
			return err
		}
	}

	for name, fragment := range fragments {
		markup, err := render(fragment)
		if err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}

		if err := write(filepath.Join(dir, name), fmt.Sprintf(fragmentShell, markup)); err != nil {
			return err
		}
	}

	return nil
}

// signedIn is the context of a page that somebody reads while signed in. The
// sidebar reads the person out of it.
var signedIn = auth.WithUser(context.Background(), user.User{
	ID:       uuid.MustParse("01a0beac-c12a-7474-9a13-a077fb9162ae"),
	FullName: "Ada Lovelace",
	Email:    "ada@example.com",
})

// inShell puts a page into the sidebar, the way the server does.
func inShell(summaries []chat.Summary, current uuid.UUID, heading string, main templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return web.Shell(summaries, current, heading, true, "abc123").Render(templ.WithChildren(ctx, main), w)
	})
}

// render turns a component into markup.
func render(component templ.Component) (string, error) {
	return renderWith(signedIn, component)
}

// renderWith turns a component into markup with a context, which the header
// reads to learn who is signed in.
func renderWith(ctx context.Context, component templ.Component) (string, error) {
	var out strings.Builder

	if err := component.Render(ctx, &out); err != nil {
		return "", err
	}

	return out.String(), nil
}

// write puts the markup into a file.
func write(path, markup string) error {
	if err := os.WriteFile(path, []byte(markup), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}
