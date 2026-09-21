package chat

import (
	"context"
	"slices"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// fakeStore is the Store of this package in memory.
type fakeStore struct {
	conversations []Conversation
	participants  map[uuid.UUID][]uuid.UUID
	messages      map[uuid.UUID][]Message
	read          map[uuid.UUID]map[uuid.UUID]time.Time
	people        []user.User
}

func newFakeStore(people ...user.User) *fakeStore {
	return &fakeStore{
		participants: map[uuid.UUID][]uuid.UUID{},
		messages:     map[uuid.UUID][]Message{},
		read:         map[uuid.UUID]map[uuid.UUID]time.Time{},
		people:       people,
	}
}

func (f *fakeStore) Create(
	_ context.Context,
	subject string,
	createdBy uuid.UUID,
	participants []uuid.UUID,
	firstBody string,
) (Conversation, error) {
	conversation := Conversation{
		ID:        uuid.NewV7(),
		Subject:   subject,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}

	f.conversations = append(f.conversations, conversation)
	f.participants[conversation.ID] = slices.Clone(participants)

	if firstBody != "" {
		f.messages[conversation.ID] = []Message{{
			ID:        uuid.NewV7(),
			AuthorID:  createdBy,
			Body:      firstBody,
			CreatedAt: time.Now(),
		}}
	}

	return conversation, nil
}

func (f *fakeStore) Get(_ context.Context, id uuid.UUID) (Conversation, bool, error) {
	for _, conversation := range f.conversations {
		if conversation.ID == id {
			return conversation, true, nil
		}
	}

	return Conversation{}, false, nil
}

func (f *fakeStore) IsParticipant(_ context.Context, conversationID, userID uuid.UUID) (bool, error) {
	return slices.Contains(f.participants[conversationID], userID), nil
}

func (f *fakeStore) List(_ context.Context, userID uuid.UUID) ([]Summary, error) {
	var summaries []Summary

	for _, conversation := range f.conversations {
		if !slices.Contains(f.participants[conversation.ID], userID) {
			continue
		}

		unread := 0
		last := conversation.CreatedAt

		for _, message := range f.messages[conversation.ID] {
			if message.CreatedAt.After(last) {
				last = message.CreatedAt
			}

			seen := f.read[conversation.ID][userID]
			if message.AuthorID != userID && message.CreatedAt.After(seen) {
				unread++
			}
		}

		summaries = append(summaries, Summary{
			Conversation:  conversation,
			LastMessageAt: last,
			Unread:        unread,
		})
	}

	return summaries, nil
}

func (f *fakeStore) Messages(_ context.Context, conversationID uuid.UUID) ([]Message, error) {
	return slices.Clone(f.messages[conversationID]), nil
}

func (f *fakeStore) AddMessage(_ context.Context, conversationID, authorID uuid.UUID, body string) (Message, error) {
	message := Message{
		ID:        uuid.NewV7(),
		AuthorID:  authorID,
		Body:      body,
		CreatedAt: time.Now(),
	}

	f.messages[conversationID] = append(f.messages[conversationID], message)

	return message, nil
}

func (f *fakeStore) MarkRead(_ context.Context, conversationID, userID uuid.UUID) (time.Time, bool, error) {
	if f.read[conversationID] == nil {
		f.read[conversationID] = map[uuid.UUID]time.Time{}
	}

	previous, seen := f.read[conversationID][userID]
	f.read[conversationID][userID] = time.Now()

	return previous, seen, nil
}

func (f *fakeStore) Participants(_ context.Context, conversationID uuid.UUID) ([]user.User, error) {
	var people []user.User

	for _, person := range f.people {
		if slices.Contains(f.participants[conversationID], person.ID) {
			people = append(people, person)
		}
	}

	return people, nil
}
