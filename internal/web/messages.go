package web

import (
	"time"

	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
)

// groupGap is the longest silence inside one group of messages. A longer
// pause starts a new group, the way a phone splits a thread.
const groupGap = 15 * time.Minute

// bubble is one message as the page draws it.
type bubble struct {
	Message chat.Message

	// Mine says that the reader wrote this message, so it belongs on the
	// other side.
	Mine bool

	// ShowName marks the first message of a group from another person.
	ShowName bool

	// ShowTime marks the last message of a group, which carries the clock.
	ShowTime bool

	// DateLabel is not empty when a date line comes before this message.
	DateLabel string

	// FirstUnread marks the first message that the reader had not seen when
	// they opened the conversation. The page draws a line above it.
	FirstUnread bool
}

// bubbles turns the messages into the rows that the page draws. A group
// breaks when the writer changes, when the day changes, or when the silence
// between two messages grows past groupGap.
// The time since says when the reader last looked at the conversation. A zero
// time means they never did, and then no line is drawn.
func bubbles(messages []chat.Message, reader user.User, since time.Time) []bubble {
	rows := make([]bubble, 0, len(messages))

	marked := since.IsZero()

	for i, message := range messages {
		row := bubble{
			Message:  message,
			Mine:     message.AuthorID == reader.ID,
			ShowName: true,
			ShowTime: true,
		}

		if i > 0 {
			// gosec reads the index as unchecked, although this branch runs
			// only when a message before this one exists.
			previous := messages[i-1] //nolint:gosec // i > 0 in this branch.

			row.ShowName = startsGroup(previous, message)

			if !sameDay(previous.CreatedAt, message.CreatedAt) {
				row.DateLabel = dateLabel(message.CreatedAt)
			}
		} else {
			row.DateLabel = dateLabel(message.CreatedAt)
		}

		if i+1 < len(messages) {
			row.ShowTime = startsGroup(message, messages[i+1])
		}

		// The side already says who wrote it.
		if row.Mine {
			row.ShowName = false
		}

		// The line stands above the first message that this reader had not
		// seen. Their own messages never carry it.
		if !marked && !row.Mine && message.CreatedAt.After(since) {
			row.FirstUnread = true
			marked = true
		}

		rows = append(rows, row)
	}

	return rows
}

// startsGroup answers whether the second message opens a new group.
func startsGroup(previous, next chat.Message) bool {
	if previous.AuthorID != next.AuthorID {
		return true
	}

	if !sameDay(previous.CreatedAt, next.CreatedAt) {
		return true
	}

	return next.CreatedAt.Sub(previous.CreatedAt) > groupGap
}

// sameDay answers whether two times fall on the same day for the reader.
func sameDay(first, second time.Time) bool {
	first, second = first.Local(), second.Local()

	return first.Year() == second.Year() && first.YearDay() == second.YearDay()
}

// dateLabel writes the line that marks a change of day.
func dateLabel(at time.Time) string {
	now := time.Now()

	switch {
	case sameDay(at, now):
		return "Today"
	case sameDay(at, now.AddDate(0, 0, -1)):
		return "Yesterday"
	default:
		return at.Local().Format("2 January 2006")
	}
}
