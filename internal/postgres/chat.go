package postgres

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/postgres/db"
	"github.com/spejder/chat/internal/user"
)

// ChatStore keeps the conversations and the messages. It carries out the
// Store interface of internal/chat.
type ChatStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewChatStore builds a store on top of a pool of connections.
func NewChatStore(pool *pgxpool.Pool) *ChatStore {
	return &ChatStore{pool: pool, queries: db.New(pool)}
}

// Create writes the conversation, the people and the first message in one
// step. Either everything lands or nothing does.
func (s *ChatStore) Create(
	ctx context.Context,
	subject string,
	createdBy uuid.UUID,
	participants []uuid.UUID,
	firstBody string,
) (chat.Conversation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return chat.Conversation{}, fmt.Errorf("start the transaction: %w", err)
	}

	// A commit makes this rollback a no-op.
	defer func() { _ = tx.Rollback(ctx) }()

	queries := s.queries.WithTx(tx)

	row, err := queries.CreateConversation(ctx, db.CreateConversationParams{
		ID:        uuid.NewV7(),
		Subject:   subject,
		CreatedBy: createdBy,
	})
	if err != nil {
		return chat.Conversation{}, fmt.Errorf("write the conversation: %w", err)
	}

	for _, person := range participants {
		if err := queries.AddParticipant(ctx, db.AddParticipantParams{
			ConversationID: row.ID,
			UserID:         person,
		}); err != nil {
			return chat.Conversation{}, fmt.Errorf("add a person: %w", err)
		}
	}

	if firstBody != "" {
		if _, err := queries.CreateMessage(ctx, db.CreateMessageParams{
			ID:             uuid.NewV7(),
			ConversationID: row.ID,
			AuthorID:       createdBy,
			Body:           firstBody,
		}); err != nil {
			return chat.Conversation{}, fmt.Errorf("write the first message: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return chat.Conversation{}, fmt.Errorf("finish the transaction: %w", err)
	}

	return toConversation(row), nil
}

// Get reads one conversation.
func (s *ChatStore) Get(ctx context.Context, id uuid.UUID) (chat.Conversation, bool, error) {
	row, err := s.queries.GetConversation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return chat.Conversation{}, false, nil
		}

		return chat.Conversation{}, false, fmt.Errorf("read the conversation: %w", err)
	}

	return toConversation(row), true, nil
}

// IsParticipant answers whether a person takes part in a conversation.
func (s *ChatStore) IsParticipant(ctx context.Context, conversationID, userID uuid.UUID) (bool, error) {
	member, err := s.queries.IsParticipant(ctx, db.IsParticipantParams{
		ConversationID: conversationID,
		UserID:         userID,
	})
	if err != nil {
		return false, fmt.Errorf("read the people: %w", err)
	}

	return member, nil
}

// List reads the conversations of one person, newest first.
func (s *ChatStore) List(ctx context.Context, userID uuid.UUID) ([]chat.Summary, error) {
	rows, err := s.queries.ListConversations(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list the conversations: %w", err)
	}

	summaries := make([]chat.Summary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, chat.Summary{
			ID:            row.ID,
			Subject:       row.Subject,
			CreatedBy:     row.CreatedBy,
			CreatedAt:     row.CreatedAt,
			Others:        row.Others,
			LastMessageAt: row.LastMessageAt,
			Unread:        int(row.Unread),
		})
	}

	return summaries, nil
}

// Messages reads one conversation in time order.
func (s *ChatStore) Messages(ctx context.Context, conversationID uuid.UUID) ([]chat.Message, error) {
	rows, err := s.queries.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("read the messages: %w", err)
	}

	messages := make([]chat.Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, chat.Message{
			ID:         row.ID,
			AuthorID:   row.AuthorID,
			AuthorName: row.AuthorName,
			Body:       row.Body,
			CreatedAt:  row.CreatedAt,
		})
	}

	return messages, nil
}

// AddMessage writes one message.
func (s *ChatStore) AddMessage(ctx context.Context, conversationID, authorID uuid.UUID, body string) (chat.Message, error) {
	row, err := s.queries.CreateMessage(ctx, db.CreateMessageParams{
		ID:             uuid.NewV7(),
		ConversationID: conversationID,
		AuthorID:       authorID,
		Body:           body,
	})
	if err != nil {
		return chat.Message{}, fmt.Errorf("write the message: %w", err)
	}

	return chat.Message{
		ID:        row.ID,
		AuthorID:  row.AuthorID,
		Body:      row.Body,
		CreatedAt: row.CreatedAt,
	}, nil
}

// MarkRead notes that one person has seen the conversation up to now.
func (s *ChatStore) MarkRead(ctx context.Context, conversationID, userID uuid.UUID) error {
	if err := s.queries.MarkRead(ctx, db.MarkReadParams{
		ConversationID: conversationID,
		UserID:         userID,
	}); err != nil {
		return fmt.Errorf("note the reading: %w", err)
	}

	return nil
}

// Participants names the people in a conversation.
func (s *ChatStore) Participants(ctx context.Context, conversationID uuid.UUID) ([]user.User, error) {
	rows, err := s.queries.ListParticipants(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("read the people: %w", err)
	}

	people := make([]user.User, 0, len(rows))
	for _, row := range rows {
		people = append(people, toUser(row))
	}

	return people, nil
}

// toConversation turns a row into the type that the rest of the application
// uses.
func toConversation(row db.Conversation) chat.Conversation {
	return chat.Conversation{
		ID:        row.ID,
		Subject:   row.Subject,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
	}
}

// ChatStore must carry out the interface that internal/chat declares.
var _ chat.Store = (*ChatStore)(nil)
