package web

import (
	"testing"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
)

// TestBubbles covers the rules that shape a group of messages.
func TestBubbles(t *testing.T) {
	t.Parallel()

	reader := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace"}
	other := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper"}

	// The times sit today, so the date line says Today and the day never
	// changes inside the run.
	base := time.Now().Truncate(time.Hour).Add(-3 * time.Hour)

	message := func(author user.User, at time.Time, body string) chat.Message {
		return chat.Message{
			ID:         uuid.NewV7(),
			AuthorID:   author.ID,
			AuthorName: author.FullName,
			Body:       body,
			CreatedAt:  at,
		}
	}

	messages := []chat.Message{
		message(other, base, "First"),
		message(other, base.Add(time.Minute), "Second"),
		message(reader, base.Add(2*time.Minute), "Mine"),
		message(other, base.Add(time.Hour), "Much later"),
	}

	rows := bubbles(messages, reader, time.Time{})

	if len(rows) != len(messages) {
		t.Fatalf("the list holds %d rows, want %d", len(rows), len(messages))
	}

	// The first message of the other person opens a group and carries a name.
	if !rows[0].ShowName || rows[0].Mine {
		t.Errorf("the first row is %+v, want a name and another writer", rows[0])
	}

	// The second message continues the group, so it carries no name. It also
	// closes the group, because the reader writes next, so it carries the
	// time.
	if rows[1].ShowName || !rows[1].ShowTime {
		t.Errorf("the second row is %+v, want no name and a time", rows[1])
	}

	// The first row sits inside the group, so it carries no time.
	if rows[0].ShowTime {
		t.Error("the first row carries a time although the group goes on")
	}

	// A message of the reader never carries a name.
	if rows[2].ShowName || !rows[2].Mine {
		t.Errorf("the third row is %+v, want the reader without a name", rows[2])
	}

	// An hour of silence opens a new group.
	if !rows[3].ShowName || !rows[3].ShowTime {
		t.Errorf("the fourth row is %+v, want a group of its own", rows[3])
	}

	// Only the first row carries a date line, because every message is today.
	if rows[0].DateLabel != "Today" {
		t.Errorf("the first date line is %q, want %q", rows[0].DateLabel, "Today")
	}

	for i, row := range rows[1:] {
		if row.DateLabel != "" {
			t.Errorf("row %d carries the date line %q", i+1, row.DateLabel)
		}
	}
}

// TestADayChangeBreaksTheGroup makes sure that a date line appears and that
// the group does not run over midnight.
func TestADayChangeBreaksTheGroup(t *testing.T) {
	t.Parallel()

	reader := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace"}
	other := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper"}

	yesterday := time.Now().AddDate(0, 0, -1)

	messages := []chat.Message{
		{ID: uuid.NewV7(), AuthorID: other.ID, AuthorName: other.FullName, Body: "Old", CreatedAt: yesterday},
		{ID: uuid.NewV7(), AuthorID: other.ID, AuthorName: other.FullName, Body: "New", CreatedAt: time.Now()},
	}

	rows := bubbles(messages, reader, time.Time{})

	if rows[0].DateLabel != "Yesterday" {
		t.Errorf("the first date line is %q, want %q", rows[0].DateLabel, "Yesterday")
	}

	if rows[1].DateLabel != "Today" {
		t.Errorf("the second date line is %q, want %q", rows[1].DateLabel, "Today")
	}

	if !rows[1].ShowName {
		t.Error("the message of the new day continues the group of the old one")
	}

	if !rows[0].ShowTime {
		t.Error("the last message of the old day carries no time")
	}
}

// TestAnOldDateReadsAsADate covers the label away from today.
func TestAnOldDateReadsAsADate(t *testing.T) {
	t.Parallel()

	// A date far from today, so the label can never read Today or Yesterday.
	at := time.Date(2020, time.March, 2, 10, 0, 0, 0, time.Local)

	if label := dateLabel(at); label != "2 March 2020" {
		t.Errorf("the label is %q, want %q", label, "2 March 2020")
	}
}

// TestTheLineForTheUnreadMessages makes sure that the line stands above the
// first message that the reader had not seen.
func TestTheLineForTheUnreadMessages(t *testing.T) {
	t.Parallel()

	reader := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace"}
	other := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper"}

	base := time.Now().Add(-time.Hour)
	since := base.Add(30 * time.Minute)

	messages := []chat.Message{
		{ID: uuid.NewV7(), AuthorID: other.ID, AuthorName: other.FullName, Body: "Old", CreatedAt: base},
		{ID: uuid.NewV7(), AuthorID: reader.ID, Body: "Mine", CreatedAt: since.Add(time.Minute)},
		{ID: uuid.NewV7(), AuthorID: other.ID, AuthorName: other.FullName, Body: "New", CreatedAt: since.Add(2 * time.Minute)},
		{ID: uuid.NewV7(), AuthorID: other.ID, AuthorName: other.FullName, Body: "Newer", CreatedAt: since.Add(3 * time.Minute)},
	}

	rows := bubbles(messages, reader, since)

	if rows[0].FirstUnread || rows[1].FirstUnread || rows[3].FirstUnread {
		t.Error("the line stands in the wrong place")
	}

	if !rows[2].FirstUnread {
		t.Error("the line does not stand above the first message that the reader had not seen")
	}

	// A reader who never looked gets no line.
	for i, row := range bubbles(messages, reader, time.Time{}) {
		if row.FirstUnread {
			t.Errorf("row %d carries the line although the reader never looked", i)
		}
	}
}
