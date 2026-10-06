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
	conversation := conversationOf(t, path).String()

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
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
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

// TestTypingRingsTheOthers makes sure that the typing route reaches the
// stream of the other person with the name of the writer, and that a
// stranger cannot use it.
func TestTypingRingsTheOthers(t *testing.T) {
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

	alan, err := users.Create(t.Context(), "Alan Turing", "alan@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the stranger: %v", err)
	}

	adaSession := signIn(t, handler, messages, ada)
	graceSession := signIn(t, handler, messages, grace)
	alanSession := signIn(t, handler, messages, alan)

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)

	path := started.Header().Get("Location")

	if answer := postAs(t, handler, path+"/typing", nil, alanSession); answer.Code != http.StatusNotFound {
		t.Errorf("a stranger typing: status = %d, want %d", answer.Code, http.StatusNotFound)
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}

	request.Header.Set("Accept", "text/event-stream")
	request.AddCookie(graceSession)

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("open the stream: %v", err)
	}

	defer func() { _ = response.Body.Close() }()

	lines := make(chan string)

	go func() {
		defer close(lines)

		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	typing := false

	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("the stream ended before it rang")
			}

			if line == "event: typing" {
				typing = true

				continue
			}

			if typing && strings.HasPrefix(line, "data: ") {
				if !strings.Contains(line, `"name":"Ada Lovelace"`) {
					t.Errorf("the typing event reads %s, want the name of the writer", line)
				}

				return
			}
		case <-tick.C:
			if answer := postAs(t, handler, path+"/typing", nil, adaSession); answer.Code != http.StatusNoContent {
				t.Fatalf("typing: status = %d, want %d", answer.Code, http.StatusNoContent)
			}
		case <-ctx.Done():
			t.Fatal("the stream never said that somebody writes")
		}
	}
}
