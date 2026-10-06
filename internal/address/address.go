// Package address writes and reads the address of a conversation. It is the
// one place that knows the form, so the pages, the notifications, the SMS
// and the routes always agree.
//
// The address is /c/ and the UUID of the conversation in base62, for
// example /c/1FzHq0mP3kXv9wQ2LrT8sB. It is 30 characters shorter than
// /conversations/ and the UUID with dashes, which matters in an SMS. The
// database keeps the UUID.
package address

import (
	"errors"
	"uuid"

	"github.com/spejder/chat/internal/base62"
)

// ErrNoConversation says that a value is no identifier of a conversation.
var ErrNoConversation = errors.New("not the identifier of a conversation")

// Conversation returns the address of the page of a conversation.
func Conversation(id uuid.UUID) string {
	return "/c/" + base62.Encode(id)
}

// Messages returns the address of the newest messages of a conversation.
func Messages(id uuid.UUID) string {
	return Conversation(id) + "/messages"
}

// Older returns the address of the messages before the newest page.
func Older(id uuid.UUID) string {
	return Conversation(id) + "/older"
}

// Typing returns the address that says that somebody writes.
func Typing(id uuid.UUID) string {
	return Conversation(id) + "/typing"
}

// ConversationID reads the identifier in an address. It takes the base62
// form and the UUID with dashes, which the old addresses
// /conversations/<uuid> carry.
func ConversationID(value string) (uuid.UUID, error) {
	if len(value) == base62.Length {
		b, err := base62.Decode(value)
		if err != nil {
			return uuid.Nil(), ErrNoConversation
		}

		return uuid.UUID(b), nil
	}

	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil(), ErrNoConversation
	}

	return id, nil
}
