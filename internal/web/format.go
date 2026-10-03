// Package web renders the pages and the fragments of the application.
package web

import (
	"strings"
	"time"

	"github.com/spejder/chat/internal/chat"
)

// listTime writes the time of the newest message for the sidebar, the way a
// phone does: the clock for today, "Yesterday", the weekday within a week,
// and the date after that.
func listTime(at, now time.Time) string {
	at, now = at.Local(), now.Local()

	switch {
	case sameDay(at, now):
		return at.Format("15:04")
	case sameDay(at, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case now.Sub(at) < 6*24*time.Hour:
		return at.Format("Monday")
	case at.Year() == now.Year():
		return at.Format("2 Jan")
	default:
		return at.Format("2 Jan 2006")
	}
}

// preview writes the newest message of a conversation in one short line,
// with the first name of the writer in front, or "You" for the reader.
func preview(summary chat.Summary) string {
	if summary.LastBody == "" {
		return "No messages yet"
	}

	writer := "You"

	if !summary.LastMine {
		if parts := splitName(summary.LastAuthor); len(parts) > 0 {
			writer = parts[0]
		}
	}

	return writer + ": " + strings.Join(strings.Fields(summary.LastBody), " ")
}

// clock writes the time of day. A bubble uses it, because the date line above
// already says which day it is.
func clock(at time.Time) string {
	return at.Local().Format("15:04")
}

// fullTime writes the whole moment, which every bubble carries in its title.
func fullTime(at time.Time) string {
	return at.Local().Format("2 January 2006 at 15:04")
}

// splitName cuts a full name into its parts.
func splitName(name string) []string {
	return strings.Fields(name)
}
