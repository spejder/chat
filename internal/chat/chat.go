// Package chat holds the conversations and the rules around them. It knows no
// SQL and no HTTP.
//
// A conversation carries a subject and a fixed set of people. Only those
// people can read it or write in it.
package chat

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// The limits of one conversation.
const (
	// MaxSubject is the longest subject line.
	MaxSubject = 200

	// MaxBody is the longest message.
	MaxBody = 4000

	// MessagePage is how many messages a page holds. The reader asks for the
	// older ones when they want them.
	MessagePage = 50
)

var (
	// ErrNotFound says that the conversation does not exist, or that this
	// person does not take part in it. The two cases give the same answer, so
	// the page never says which conversations exist.
	ErrNotFound = errors.New("conversation not found")

	// ErrNoSubject says that the subject is empty.
	ErrNoSubject = errors.New("write a subject")

	// ErrNoParticipants says that nobody else is in the conversation.
	ErrNoParticipants = errors.New("choose at least one other person")

	// ErrEmptyMessage says that the message is empty.
	ErrEmptyMessage = errors.New("write a message")

	// ErrTooLong says that the text is over the limit.
	ErrTooLong = errors.New("the text is too long")
)

// Conversation is a subject with a fixed set of people.
type Conversation struct {
	ID        uuid.UUID
	Subject   string
	CreatedBy uuid.UUID
	CreatedAt time.Time
}

// Summary is one line of the list page.
type Summary struct {
	Conversation

	// Others names the people besides the reader, in one string.
	Others string

	// OtherNames lists the people besides the reader, in name order.
	OtherNames []string

	// LastMessageAt is the time of the newest message, or the time the
	// conversation started when it holds none.
	LastMessageAt time.Time

	// LastAuthor and LastBody are the writer and the text of the newest
	// message. LastMine says that the reader wrote it. Both strings are
	// empty when the conversation holds no message.
	LastAuthor string
	LastBody   string
	LastMine   bool

	// LastRead says that the reader wrote the newest message and that every
	// other person has read it since.
	LastRead bool

	// Unread counts the messages from other people that the reader has not
	// seen.
	Unread int
}

// Message is one line of a conversation.
type Message struct {
	ID         uuid.UUID
	AuthorID   uuid.UUID
	AuthorName string
	Body       string
	CreatedAt  time.Time
}

// Reader is one person in a conversation with the time they last read it.
// The zero time means they never opened it.
type Reader struct {
	ID         uuid.UUID
	Name       string
	LastReadAt time.Time
}

// Store keeps the conversations. The store in internal/postgres carries it
// out. A lookup that finds nothing returns false, not an error.
type Store interface {
	Create(ctx context.Context, subject string, createdBy uuid.UUID, participants []uuid.UUID, firstBody string) (Conversation, error)
	Get(ctx context.Context, id uuid.UUID) (Conversation, bool, error)
	IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error)
	List(ctx context.Context, userID uuid.UUID) ([]Summary, error)
	Messages(ctx context.Context, conversationID uuid.UUID, limit int) ([]Message, error)
	MessagesBefore(ctx context.Context, conversationID, before uuid.UUID, limit int) ([]Message, error)
	AddMessage(ctx context.Context, conversationID, authorID uuid.UUID, body string) (Message, error)
	// MarkRead notes the reading and returns the time it replaces. The
	// second value is false when that person had read nothing yet.
	MarkRead(ctx context.Context, conversationID, userID uuid.UUID) (time.Time, bool, error)
	Participants(ctx context.Context, conversationID uuid.UUID) ([]user.User, error)
	Readers(ctx context.Context, conversationID uuid.UUID) ([]Reader, error)
}

// Notifier hears about every new message, so the people of the conversation
// learn about it with the page closed. internal/push carries it out. The
// call must return at once, because the writer waits for the answer.
type Notifier interface {
	MessageWritten(ctx context.Context, conversation Conversation, message Message, recipients []uuid.UUID)
}

// Broadcaster tells the open pages of some people that a conversation
// changed: a new message, or a read that moves the read mark and the unread
// count. The page then asks for the conversation with the version it holds.
// internal/postgres carries it out through LISTEN and NOTIFY, so every
// server instance hears it.
type Broadcaster interface {
	Changed(ctx context.Context, conversationID uuid.UUID, people []uuid.UUID)

	// Typing tells the pages of the people that a person writes in the
	// conversation right now. The page shows it for a few seconds.
	Typing(ctx context.Context, conversationID uuid.UUID, writer user.User, people []uuid.UUID)
}
