package address_test

import (
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/spejder/chat/internal/address"
)

// TestBothFormsReadTheSameConversation makes sure that the short address
// and the old UUID read back to the same identifier.
func TestBothFormsReadTheSameConversation(t *testing.T) {
	t.Parallel()

	id := uuid.NewV7()

	page := address.Conversation(id)
	if !strings.HasPrefix(page, "/c/") || len(page) != len("/c/")+22 {
		t.Fatalf("Conversation = %q, want /c/ and 22 characters", page)
	}

	for _, value := range []string{strings.TrimPrefix(page, "/c/"), id.String()} {
		got, err := address.ConversationID(value)
		if err != nil || got != id {
			t.Errorf("ConversationID(%q) = %v, %v, want %v", value, got, err, id)
		}
	}

	if got := address.Messages(id); got != page+"/messages" {
		t.Errorf("Messages = %q, want %q", got, page+"/messages")
	}

	for _, value := range []string{"", "list", "new", "zzzzzzzzzzzzzzzzzzzzzz", "not-a-uuid-at-all"} {
		if _, err := address.ConversationID(value); !errors.Is(err, address.ErrNoConversation) {
			t.Errorf("ConversationID(%q) = %v, want %v", value, err, address.ErrNoConversation)
		}
	}
}
