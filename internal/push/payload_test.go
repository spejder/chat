package push

import (
	"encoding/json"
	"strings"
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

	got := payloadFor(conversation, message, 4, false)

	want := Payload{
		Title:  "Lunch",
		Body:   "Grace: Are you in?",
		URL:    address.Conversation(conversation.ID),
		Tag:    "conversation-" + conversation.ID.String(),
		Unread: 4,
	}

	if got != want {
		t.Errorf("payload = %+v, want %+v", got, want)
	}
}

// TestTheNightIsQuiet makes sure that a payload in the night carries the
// mark for the worker, and a payload by day leaves it out of the JSON.
func TestTheNightIsQuiet(t *testing.T) {
	t.Parallel()

	conversation := chat.Conversation{ID: uuid.NewV7(), Subject: "Lunch"}
	message := chat.Message{AuthorName: "Grace Hopper", Body: "Still up?"}

	night, err := json.Marshal(payloadFor(conversation, message, 1, true))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if !strings.Contains(string(night), `"quiet":true`) {
		t.Errorf("the night payload reads %s, want quiet", night)
	}

	day, err := json.Marshal(payloadFor(conversation, message, 1, false))
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	if strings.Contains(string(day), "quiet") {
		t.Errorf("the day payload reads %s, want no quiet", day)
	}
}
