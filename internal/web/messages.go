package web

import (
	"strings"
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

	// StartsGroup marks the first message of a group, whoever wrote it. The
	// page puts more room above such a message than inside a group.
	StartsGroup bool

	// ShowName marks the first message of a group from another person.
	ShowName bool

	// ShowTime marks the last message of a group, which carries the clock.
	ShowTime bool

	// DateLabel is not empty when a date line comes before this message.
	DateLabel string

	// FirstUnread marks the first message that the reader had not seen when
	// they opened the conversation. The page draws a line above it.
	FirstUnread bool

	// ReadMark is not empty on the newest message of the reader when
	// somebody else has read it.
	ReadMark string
}

// Panel holds what the message list needs besides the messages themselves.
type Panel struct {
	// Reader is the person who looks at the page.
	Reader user.User

	// Since is the moment that person last looked. A zero time means never.
	Since time.Time

	// Readers are the people of the conversation with their reading times.
	Readers []chat.Reader

	// People counts everybody in the conversation. With two of them the
	// names above the groups disappear, because the side says who wrote it.
	People int

	// History marks an older block, which the reader asked for. Such a block
	// carries neither the line for the unread messages nor the read mark,
	// because both belong to the newest page.
	History bool
}

// bubbles turns the messages into the rows that the page draws. A group
// breaks when the writer changes, when the day changes, or when the silence
// between two messages grows past groupGap.
func bubbles(messages []chat.Message, panel Panel) []bubble {
	rows := make([]bubble, 0, len(messages))

	reader := panel.Reader
	since := panel.Since
	marked := since.IsZero() || panel.History

	for i, message := range messages {
		row := bubble{
			Message:     message,
			Mine:        message.AuthorID == reader.ID,
			StartsGroup: true,
			ShowName:    true,
			ShowTime:    true,
		}

		if i > 0 {
			// gosec reads the index as unchecked, although this branch runs
			// only when a message before this one exists.
			previous := messages[i-1] //nolint:gosec // i > 0 in this branch.

			row.StartsGroup = startsGroup(previous, message)
			row.ShowName = row.StartsGroup

			if !sameDay(previous.CreatedAt, message.CreatedAt) {
				row.DateLabel = dateLabel(message.CreatedAt)
			}
		} else {
			row.DateLabel = dateLabel(message.CreatedAt)
		}

		if i+1 < len(messages) {
			row.ShowTime = startsGroup(message, messages[i+1])
		}

		// The side already says who wrote it, and in a conversation of two
		// there is nobody else to name.
		if row.Mine || panel.People <= 2 {
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

	if !panel.History {
		markRead(rows, panel)
	}

	return rows
}

// markRead writes the mark on the newest message of the reader, the way a
// phone does. Nothing is marked when nobody else has read it yet.
func markRead(rows []bubble, panel Panel) {
	newest := -1

	for i, row := range rows {
		if row.Mine {
			newest = i
		}
	}

	if newest < 0 {
		return
	}

	written := rows[newest].Message.CreatedAt

	var (
		others int
		names  []string
	)

	for _, reader := range panel.Readers {
		if reader.ID == panel.Reader.ID {
			continue
		}

		others++

		if reader.LastReadAt.After(written) {
			names = append(names, reader.Name)
		}
	}

	switch {
	case len(names) == 0:
		return
	case len(names) == others:
		rows[newest].ReadMark = "Read"
	default:
		rows[newest].ReadMark = "Read by " + strings.Join(names, ", ")
	}
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
