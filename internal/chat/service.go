package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// Service holds the rules of a conversation.
type Service struct {
	store Store
}

// New builds the service.
func New(store Store) *Service {
	return &Service{store: store}
}

// Start opens a conversation. The person who starts it takes part in it, and
// so does everybody in others.
func (s *Service) Start(ctx context.Context, creator user.User, subject string, others []uuid.UUID, firstBody string) (Conversation, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Conversation{}, ErrNoSubject
	}

	if utf8.RuneCountInString(subject) > MaxSubject {
		return Conversation{}, ErrTooLong
	}

	body, err := cleanBody(firstBody)
	if err != nil {
		return Conversation{}, err
	}

	participants := withoutRepeats(others, creator.ID)
	if len(participants) == 0 {
		return Conversation{}, ErrNoParticipants
	}

	// The person who writes the first message must be in the conversation.
	participants = append(participants, creator.ID)

	conversation, err := s.store.Create(ctx, subject, creator.ID, participants, body)
	if err != nil {
		return Conversation{}, fmt.Errorf("start the conversation: %w", err)
	}

	return conversation, nil
}

// List reads the conversations of one person, newest first.
func (s *Service) List(ctx context.Context, person user.User) ([]Summary, error) {
	summaries, err := s.store.List(ctx, person.ID)
	if err != nil {
		return nil, fmt.Errorf("list the conversations: %w", err)
	}

	return summaries, nil
}

// Opened is a conversation as the page draws it the first time.
type Opened struct {
	Conversation Conversation

	// Messages is the newest page of the conversation.
	Messages []Message

	// Since is the moment this person last looked. The page draws the line
	// for the unread messages from it, and it is the zero time the first time
	// somebody opens the conversation.
	Since time.Time

	// HasOlder says that messages exist in front of this page.
	HasOlder bool
}

// Read returns the newest part of a conversation, and notes that this person
// has seen it.
func (s *Service) Read(ctx context.Context, person user.User, id uuid.UUID) (Opened, error) {
	conversation, err := s.find(ctx, person, id)
	if err != nil {
		return Opened{}, err
	}

	messages, more, err := s.page(ctx, id)
	if err != nil {
		return Opened{}, err
	}

	since, _, err := s.store.MarkRead(ctx, id, person.ID)
	if err != nil {
		return Opened{}, fmt.Errorf("note the reading: %w", err)
	}

	return Opened{Conversation: conversation, Messages: messages, Since: since, HasOlder: more}, nil
}

// Messages returns the newest part of a conversation, which is what the page
// asks for every few seconds. It also notes that this person has seen it.
func (s *Service) Messages(ctx context.Context, person user.User, id uuid.UUID) ([]Message, error) {
	if _, err := s.find(ctx, person, id); err != nil {
		return nil, err
	}

	messages, _, err := s.page(ctx, id)
	if err != nil {
		return nil, err
	}

	if _, _, err := s.store.MarkRead(ctx, id, person.ID); err != nil {
		return nil, fmt.Errorf("note the reading: %w", err)
	}

	return messages, nil
}

// Older returns the part in front of one message, which the reader asks for
// with the button above the conversation. The second value says whether even
// older messages exist.
func (s *Service) Older(ctx context.Context, person user.User, id, before uuid.UUID) ([]Message, bool, error) {
	if _, err := s.find(ctx, person, id); err != nil {
		return nil, false, err
	}

	messages, err := s.store.MessagesBefore(ctx, id, before, MessagePage+1)
	if err != nil {
		return nil, false, fmt.Errorf("read the older messages: %w", err)
	}

	return cutPage(messages)
}

// page reads the newest messages and says whether older ones exist.
func (s *Service) page(ctx context.Context, id uuid.UUID) ([]Message, bool, error) {
	messages, err := s.store.Messages(ctx, id, MessagePage+1)
	if err != nil {
		return nil, false, fmt.Errorf("read the messages: %w", err)
	}

	return cutPage(messages)
}

// cutPage asks for one message more than a page holds, which answers whether
// more exist, and hands back the page itself.
func cutPage(messages []Message) ([]Message, bool, error) {
	if len(messages) <= MessagePage {
		return messages, false, nil
	}

	// The extra message is the oldest one, which the store puts first.
	return messages[len(messages)-MessagePage:], true, nil
}

// Write adds a message and returns the conversation as it now stands.
func (s *Service) Write(ctx context.Context, person user.User, id uuid.UUID, body string) ([]Message, error) {
	if _, err := s.find(ctx, person, id); err != nil {
		return nil, err
	}

	clean, err := cleanBody(body)
	if err != nil {
		return nil, err
	}

	if _, err := s.store.AddMessage(ctx, id, person.ID, clean); err != nil {
		return nil, fmt.Errorf("write the message: %w", err)
	}

	return s.Messages(ctx, person, id)
}

// Participants names the people in a conversation.
func (s *Service) Participants(ctx context.Context, person user.User, id uuid.UUID) ([]user.User, error) {
	if _, err := s.find(ctx, person, id); err != nil {
		return nil, err
	}

	people, err := s.store.Participants(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read the people: %w", err)
	}

	return people, nil
}

// Readers gives the people of a conversation with the time each of them last
// read it. The page marks a message as read from these times.
func (s *Service) Readers(ctx context.Context, person user.User, id uuid.UUID) ([]Reader, error) {
	if _, err := s.find(ctx, person, id); err != nil {
		return nil, err
	}

	readers, err := s.store.Readers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read the reading times: %w", err)
	}

	return readers, nil
}

// find reads a conversation that this person takes part in. A conversation
// that does not exist and one that belongs to other people give the same
// answer.
func (s *Service) find(ctx context.Context, person user.User, id uuid.UUID) (Conversation, error) {
	member, err := s.store.IsParticipant(ctx, id, person.ID)
	if err != nil {
		return Conversation{}, fmt.Errorf("read the people: %w", err)
	}

	if !member {
		return Conversation{}, ErrNotFound
	}

	conversation, ok, err := s.store.Get(ctx, id)
	if err != nil {
		return Conversation{}, fmt.Errorf("read the conversation: %w", err)
	}

	if !ok {
		return Conversation{}, ErrNotFound
	}

	return conversation, nil
}

// cleanBody trims a message and checks its size.
func cleanBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrEmptyMessage
	}

	if utf8.RuneCountInString(body) > MaxBody {
		return "", ErrTooLong
	}

	return body, nil
}

// withoutRepeats drops the empty identifier, the repeats and the person who
// starts the conversation, because that person comes in separately.
func withoutRepeats(ids []uuid.UUID, creator uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))

	for _, id := range ids {
		if id == uuid.Nil() || id == creator || slices.Contains(out, id) {
			continue
		}

		out = append(out, id)
	}

	return out
}
