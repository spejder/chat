package push

import (
	"testing"
	"uuid"

	"github.com/spejder/chat/internal/address"

	"github.com/spejder/chat/internal/chat"
)

// TestThePayloadCarriesTheCount makes sure that the message for one person
// holds the count of that person, which the worker puts on the icon.
func TestThePayloadCarriesTheCount(t *testing.T) {
	t.Parallel()

	conversation := chat.Conversation{ID: uuid.NewV7(), Subject: "Lunch"}
	message := chat.Message{AuthorName: "Grace Hopper", Body: "Are\nyou in?"}

	got := payloadFor(conversation, message, 4)

	want := Payload{
		Title:  "Lunch",
		Body:   "Grace Hopper: Are you in?",
		URL:    address.Conversation(conversation.ID),
		Tag:    "conversation-" + conversation.ID.String(),
		Unread: 4,
	}

	if got != want {
		t.Errorf("payload = %+v, want %+v", got, want)
	}
}
