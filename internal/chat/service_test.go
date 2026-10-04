package chat

import (
	"context"
	"errors"
	"slices"
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

			service := New(newFakeStore(ada, grace), nil, nil)

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
	service := New(newFakeStore(ada, grace), nil, nil)

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

	service := New(newFakeStore(ada, grace, stranger), nil, nil)

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
	service := New(newFakeStore(ada, grace), nil, nil)

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

// notice is one call to the notifier.
type notice struct {
	conversation Conversation
	message      Message
	recipients   []uuid.UUID
}

// recorder is a notifier that keeps every call.
type recorder struct {
	notices []notice
}

func (r *recorder) MessageWritten(_ context.Context, conversation Conversation, message Message, recipients []uuid.UUID) {
	r.notices = append(r.notices, notice{conversation: conversation, message: message, recipients: recipients})
}

// TestTheOthersHearAboutAMessage makes sure that every new message reaches
// the notifier, with the other people and never the writer.
func TestTheOthersHearAboutAMessage(t *testing.T) {
	t.Parallel()

	ada, grace := people()
	alan := user.User{ID: uuid.NewV7(), FullName: "Alan Turing"}

	heard := &recorder{}
	service := New(newFakeStore(ada, grace, alan), heard, nil)

	conversation, err := service.Start(t.Context(), ada, "Lunch", []uuid.UUID{grace.ID, alan.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if _, err := service.Write(t.Context(), grace, conversation.ID, "I am in"); err != nil {
		t.Fatalf("write: %v", err)
	}

	if len(heard.notices) != 2 {
		t.Fatalf("the notifier heard %d messages, want 2", len(heard.notices))
	}

	tests := []struct {
		name   string
		notice notice
		writer user.User
		body   string
		want   []uuid.UUID
	}{
		{name: "the first message", notice: heard.notices[0], writer: ada, body: "Are you in?", want: []uuid.UUID{grace.ID, alan.ID}},
		{name: "an answer", notice: heard.notices[1], writer: grace, body: "I am in", want: []uuid.UUID{ada.ID, alan.ID}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if test.notice.conversation.ID != conversation.ID || test.notice.conversation.Subject != "Lunch" {
				t.Errorf("the conversation is %+v, want Lunch", test.notice.conversation)
			}

			if test.notice.message.AuthorName != test.writer.FullName || test.notice.message.Body != test.body {
				t.Errorf("the message is %+v, want %q by %s", test.notice.message, test.body, test.writer.FullName)
			}

			got := slices.Clone(test.notice.recipients)
			slices.SortFunc(got, uuid.UUID.Compare)

			want := slices.Clone(test.want)
			slices.SortFunc(want, uuid.UUID.Compare)

			if !slices.Equal(got, want) {
				t.Errorf("the recipients are %v, want %v", got, want)
			}
		})
	}
}

// bell is a broadcaster that keeps every call.
type bell struct {
	rings  []notice
	typing []notice
}

func (b *bell) Changed(_ context.Context, id uuid.UUID, people []uuid.UUID) {
	b.rings = append(b.rings, notice{conversation: Conversation{ID: id}, recipients: people})
}

func (b *bell) Typing(_ context.Context, id uuid.UUID, writer user.User, people []uuid.UUID) {
	b.typing = append(b.typing, notice{
		conversation: Conversation{ID: id},
		message:      Message{AuthorID: writer.ID, AuthorName: writer.FullName},
		recipients:   people,
	})
}

// TestTypingReachesOnlyTheOthers makes sure that the writer never hears about
// their own typing, and that a stranger cannot ring the bell.
func TestTypingReachesOnlyTheOthers(t *testing.T) {
	t.Parallel()

	ada, grace := people()
	stranger := user.User{ID: uuid.NewV7(), FullName: "Alan Turing"}

	rang := &bell{}
	service := New(newFakeStore(ada, grace, stranger), nil, rang)

	conversation, err := service.Start(t.Context(), ada, "Lunch", []uuid.UUID{grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := service.Typing(t.Context(), grace, conversation.ID); err != nil {
		t.Fatalf("typing: %v", err)
	}

	if len(rang.typing) != 1 {
		t.Fatalf("the bell rang %d times for typing, want 1", len(rang.typing))
	}

	if got := rang.typing[0]; got.message.AuthorName != grace.FullName || !slices.Equal(got.recipients, []uuid.UUID{ada.ID}) {
		t.Errorf("typing rang for %+v, want Grace to Ada", got)
	}

	if err := service.Typing(t.Context(), stranger, conversation.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger typing gave %v, want %v", err, ErrNotFound)
	}
}

// TestTheOpenPagesHearAboutChanges makes sure that a message and a read of
// something new reach the broadcaster, and that a read of nothing new stays
// silent, so two open pages never wake each other in a loop.
func TestTheOpenPagesHearAboutChanges(t *testing.T) {
	t.Parallel()

	ada, grace := people()

	rang := &bell{}
	service := New(newFakeStore(ada, grace), nil, rang)

	conversation, err := service.Start(t.Context(), ada, "Lunch", []uuid.UUID{grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	everybody := []uuid.UUID{ada.ID, grace.ID}

	check := func(step string, want int) {
		t.Helper()

		if len(rang.rings) != want {
			t.Fatalf("%s: the bell rang %d times, want %d", step, len(rang.rings), want)
		}

		last := rang.rings[len(rang.rings)-1]

		got := slices.Clone(last.recipients)
		slices.SortFunc(got, uuid.UUID.Compare)

		wanted := slices.Clone(everybody)
		slices.SortFunc(wanted, uuid.UUID.Compare)

		if last.conversation.ID != conversation.ID || !slices.Equal(got, wanted) {
			t.Errorf("%s: the bell rang for %v and %v, want the conversation and both people", step, last.conversation.ID, got)
		}
	}

	check("start", 1)

	// Grace reads the first message, which is new to her.
	if _, err := service.Read(t.Context(), grace, conversation.ID); err != nil {
		t.Fatalf("read: %v", err)
	}

	check("the first read", 2)

	// The poll of her page finds nothing new and stays silent.
	if _, err := service.Messages(t.Context(), grace, conversation.ID); err != nil {
		t.Fatalf("messages: %v", err)
	}

	if len(rang.rings) != 2 {
		t.Fatalf("a read of nothing new rang the bell: %d rings, want 2", len(rang.rings))
	}

	// Her answer rings for both, and the read of the writer covers nothing
	// new to her.
	if _, err := service.Write(t.Context(), grace, conversation.ID, "I am in"); err != nil {
		t.Fatalf("write: %v", err)
	}

	check("the answer", 3)
}
