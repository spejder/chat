// Command renderhtml writes the pages and the fragments of the application
// into a directory, so an HTML validator can read them as files.
//
// Usage: go run ./cmd/renderhtml [directory]
package main

import (
	"context"
	"fmt"
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

	messages := []chat.Message{
		{ID: uuid.NewV7(), AuthorID: people[0].ID, AuthorName: people[0].FullName, Body: "Are you in?", CreatedAt: at},
	}

	pages := map[string]templ.Component{
		"home.html":             web.Home(),
		"login.html":            web.Login(web.EmailPanel("", "")),
		"conversations.html":    web.Conversations([]chat.Summary{{Conversation: conversation, Others: "Grace Hopper", LastMessageAt: at, Unread: 2}}),
		"conversation.html":     web.ConversationPage(conversation, people, messages, people[0]),
		"new-conversation.html": web.NewConversation(people, "Lunch", "Are you in?", ""),
	}

	fragments := map[string]templ.Component{
		"greeting.html":      web.Greeting(at),
		"login-code.html":    web.CodePanel("ada@example.com", "That code is wrong. Try again."),
		"login-passkey.html": web.PasskeyPanel("ada@example.com", `{"publicKey":{}}`, "01a0beac-c12a-7474-9a13-a077fb9162ad"),
		"login-offer.html":   web.PasskeyOffer(`{"publicKey":{}}`, "01a0beac-c12a-7474-9a13-a077fb9162ad"),
	}

	// The page with somebody signed in shows the header with the name.
	signedIn := auth.WithUser(context.Background(), user.User{FullName: "Ada Lovelace", Email: "ada@example.com"})

	markup, err := renderWith(signedIn, web.Home())
	if err != nil {
		return fmt.Errorf("render home-signed-in.html: %w", err)
	}

	if err := write(filepath.Join(dir, "home-signed-in.html"), markup); err != nil {
		return err
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

// render turns a component into markup.
func render(component templ.Component) (string, error) {
	return renderWith(context.Background(), component)
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
