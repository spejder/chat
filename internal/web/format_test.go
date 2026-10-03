package web

import (
	"testing"
	"time"

	"github.com/spejder/chat/internal/chat"
)

// TestListTime covers the time beside a line of the sidebar.
func TestListTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 3, 14, 0, 0, 0, time.Local)

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "today", at: now.Add(-2 * time.Hour), want: "12:00"},
		{name: "yesterday", at: now.AddDate(0, 0, -1), want: "Yesterday"},
		{name: "this week", at: now.AddDate(0, 0, -3), want: "Wednesday"},
		{name: "this year", at: now.AddDate(0, -2, 0), want: "3 Aug"},
		{name: "an older year", at: now.AddDate(-1, 0, 0), want: "3 Oct 2025"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := listTime(test.at, now); got != test.want {
				t.Errorf("listTime = %q, want %q", got, test.want)
			}
		})
	}
}

// TestPreview covers the line under the subject.
func TestPreview(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		summary chat.Summary
		want    string
	}{
		{name: "no message", summary: chat.Summary{}, want: "No messages yet"},
		{name: "somebody else", summary: chat.Summary{LastAuthor: "Grace Hopper", LastBody: "Are you in?"}, want: "Grace: Are you in?"},
		{name: "the reader", summary: chat.Summary{LastAuthor: "Ada Lovelace", LastBody: "Yes", LastMine: true}, want: "You: Yes"},
		{name: "more lines", summary: chat.Summary{LastAuthor: "Grace Hopper", LastBody: "One\n\ntwo"}, want: "Grace: One two"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := preview(test.summary); got != test.want {
				t.Errorf("preview = %q, want %q", got, test.want)
			}
		})
	}
}
