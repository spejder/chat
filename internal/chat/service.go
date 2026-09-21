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

// Read returns a conversation with its messages, and notes that this person
// has seen it.
//
// The third value is the moment this person last looked at the conversation.
// The page draws the line for the unread messages from it. It is the zero
// time when this person opens the conversation for the first time.
func (s *Service) Read(ctx context.Context, person user.User, id uuid.UUID) (Conversation, []Message, time.Time, error) {
	conversation, err := s.find(ctx, person, id)
	if err != nil {
		return Conversation{}, nil, time.Time{}, err
	}

	messages, err := s.store.Messages(ctx, id)
	if err != nil {
		return Conversation{}, nil, time.Time{}, fmt.Errorf("read the messages: %w", err)
	}

	since, _, err := s.store.MarkRead(ctx, id, person.ID)
	if err != nil {
		return Conversation{}, nil, time.Time{}, fmt.Errorf("note the reading: %w", err)
	}

	return conversation, messages, since, nil
}

// Messages returns the messages alone, which is what the page asks for every
// few seconds. It also notes that this person has seen them.
func (s *Service) Messages(ctx context.Context, person user.User, id uuid.UUID) ([]Message, error) {
	if _, err := s.find(ctx, person, id); err != nil {
		return nil, err
	}

	messages, err := s.store.Messages(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read the messages: %w", err)
	}

	if _, _, err := s.store.MarkRead(ctx, id, person.ID); err != nil {
		return nil, fmt.Errorf("note the reading: %w", err)
	}

	return messages, nil
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
