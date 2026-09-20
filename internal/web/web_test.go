package web_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/web"
)

// TestGreetingShowsTheTime makes sure that the fragment prints the time it
// receives, because the changing text is the visible proof of the swap.
func TestGreetingShowsTheTime(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.September, 20, 13, 45, 7, 0, time.UTC)

	var out strings.Builder
	if err := web.Greeting(at).Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(out.String(), "13:45:07") {
		t.Errorf("the fragment does not hold the time: %s", out.String())
	}
}

// TestHomeIsAWholeDocument makes sure that the page carries the shell from the
// layout.
func TestHomeIsAWholeDocument(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	if err := web.Home().Render(context.Background(), &out); err != nil {
		t.Fatalf("render: %v", err)
	}

	body := out.String()
	for _, want := range []string{
		"<!doctype html>",
		"<title>Chat</title>",
		`<meta name="description"`,
		`<link rel="icon"`,
		"</html>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not hold %q", want)
		}
	}
}

// TestLayoutPutsTheChildrenInTheBody makes sure that a page can place its own
// markup inside the shell.
func TestLayoutPutsTheChildrenInTheBody(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	if err := web.Layout("Title", "Description").Render(
		templ.WithChildren(context.Background(), templ.Raw("<span>marker</span>")),
		&out,
	); err != nil {
		t.Fatalf("render: %v", err)
	}

	body := out.String()
	if !strings.Contains(body, "<title>Title</title>") {
		t.Errorf("the shell does not hold the title: %s", body)
	}

	_, after, found := strings.Cut(body, "<body")
	if !found || !strings.Contains(after, "marker") {
		t.Errorf("the children are not inside the body: %s", body)
	}
}
