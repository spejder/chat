package server_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestTheStreamNeedsASession makes sure that a visitor gets no stream.
func TestTheStreamNeedsASession(t *testing.T) {
	t.Parallel()

	answer := get(t, newTestHandler(t), "/events", nil)
	if answer.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", answer.Code, http.StatusSeeOther)
	}
}

// TestAMessageRingsTheStream opens the stream of one person and writes in a
// conversation of that person. The stream must name the conversation.
func TestAMessageRingsTheStream(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	grace, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	adaSession := signIn(t, handler, messages, ada)
	graceSession := signIn(t, handler, messages, grace)

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)

	path := started.Header().Get("Location")
	conversation := strings.TrimPrefix(path, "/conversations/")

	// A real server, because a recorder cannot stream.
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}

	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Accept-Encoding", "gzip")
	request.AddCookie(graceSession)

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("open the stream: %v", err)
	}

	defer func() { _ = response.Body.Close() }()

	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want the stream type", got)
	}

	if got := response.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("the stream is packed with %q, want it plain", got)
	}

	lines := make(chan string)

	go func() {
		defer close(lines)

		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	// The listener of the test needs a moment before its LISTEN stands, so
	// the writer keeps writing until the stream rings.
	write := time.NewTicker(100 * time.Millisecond)
	defer write.Stop()

	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("the stream ended before it rang")
			}

			if line == "data: "+conversation {
				return
			}
		case <-write.C:
			postAs(t, handler, path+"/messages", url.Values{"body": {"Still there?"}}, adaSession)
		case <-ctx.Done():
			t.Fatal("the stream never named the conversation")
		}
	}
}
