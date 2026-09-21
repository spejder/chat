package chat

import (
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// two people for the tests.
func people() (user.User, user.User) {
	ada := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace", Email: "ada@example.com"}
	grace := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper", Email: "grace@example.com"}

	return ada, grace
}

// TestStartChecksTheInput covers every rule that stops a conversation.
func TestStartChecksTheInput(t *testing.T) {
	t.Parallel()

	ada, grace := people()

	tests := []struct {
		name    string
		subject string
		others  []uuid.UUID
		body    string
		want    error
	}{
		{name: "no subject", subject: "   ", others: []uuid.UUID{grace.ID}, body: "Hello", want: ErrNoSubject},
		{name: "a subject that is too long", subject: strings.Repeat("a", MaxSubject+1), others: []uuid.UUID{grace.ID}, body: "Hello", want: ErrTooLong},
		{name: "nobody else", subject: "Lunch", others: nil, body: "Hello", want: ErrNoParticipants},
		{name: "only myself", subject: "Lunch", others: []uuid.UUID{ada.ID}, body: "Hello", want: ErrNoParticipants},
		{name: "no message", subject: "Lunch", others: []uuid.UUID{grace.ID}, body: " ", want: ErrEmptyMessage},
		{name: "a message that is too long", subject: "Lunch", others: []uuid.UUID{grace.ID}, body: strings.Repeat("a", MaxBody+1), want: ErrTooLong},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := New(newFakeStore(ada, grace))

			_, err := service.Start(t.Context(), ada, test.subject, test.others, test.body)
			if !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}
		})
	}
}

// TestStartPutsBothPeopleIn makes sure that the writer is part of the
// conversation and that the subject arrives trimmed.
func TestStartPutsBothPeopleIn(t *testing.T) {
	t.Parallel()

	ada, grace := people()
	service := New(newFakeStore(ada, grace))

	conversation, err := service.Start(t.Context(), ada, "  Lunch  ", []uuid.UUID{grace.ID, grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if conversation.Subject != "Lunch" {
		t.Errorf("subject = %q, want %q", conversation.Subject, "Lunch")
	}

	for _, person := range []user.User{ada, grace} {
		if _, err := service.Read(t.Context(), person, conversation.ID); err != nil {
			t.Errorf("%s cannot read the conversation: %v", person.FullName, err)
		}
	}
}

// TestAStrangerGetsNothing makes sure that every way in asks for the people
// first.
func TestAStrangerGetsNothing(t *testing.T) {
	t.Parallel()

	ada, grace := people()
	stranger := user.User{ID: uuid.NewV7(), FullName: "Alan Turing"}

	service := New(newFakeStore(ada, grace, stranger))

	conversation, err := service.Start(t.Context(), ada, "Lunch", []uuid.UUID{grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := service.Read(t.Context(), stranger, conversation.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("read: error = %v, want %v", err, ErrNotFound)
	}

	if _, err := service.Messages(t.Context(), stranger, conversation.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("messages: error = %v, want %v", err, ErrNotFound)
	}

	if _, err := service.Write(t.Context(), stranger, conversation.ID, "Hello"); !errors.Is(err, ErrNotFound) {
		t.Errorf("write: error = %v, want %v", err, ErrNotFound)
	}

	if _, err := service.Participants(t.Context(), stranger, conversation.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("participants: error = %v, want %v", err, ErrNotFound)
	}

	if _, err := service.Readers(t.Context(), stranger, conversation.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("readers: error = %v, want %v", err, ErrNotFound)
	}

	if _, _, err := service.Older(t.Context(), stranger, conversation.ID, uuid.NewV7()); !errors.Is(err, ErrNotFound) {
		t.Errorf("older: error = %v, want %v", err, ErrNotFound)
	}
}

// TestWriteAddsToTheEnd makes sure that a message lands after the first one
// and that reading clears the count.
func TestWriteAddsToTheEnd(t *testing.T) {
	t.Parallel()

	ada, grace := people()
	service := New(newFakeStore(ada, grace))

	conversation, err := service.Start(t.Context(), ada, "Lunch", []uuid.UUID{grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	summaries, err := service.List(t.Context(), grace)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(summaries) != 1 || summaries[0].Unread != 1 {
		t.Fatalf("the list is %+v, want one line with one unread", summaries)
	}

	messages, err := service.Write(t.Context(), grace, conversation.ID, "  I am in  ")
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if len(messages) != 2 || messages[1].Body != "I am in" {
		t.Fatalf("the messages are %+v, want the trimmed answer last", messages)
	}

	summaries, err = service.List(t.Context(), grace)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if summaries[0].Unread != 0 {
		t.Errorf("after the read there are %d unread, want 0", summaries[0].Unread)
	}
}
