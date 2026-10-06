package remind_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/spejder/chat/internal/address"

	"github.com/spejder/chat/internal/remind"
	"github.com/spejder/chat/internal/sms"
)

// fakeStore hands out a fixed list once and notes the times it got.
type fakeStore struct {
	due                  []remind.Due
	dueBefore, notBefore time.Time
}

func (f *fakeStore) Claim(_ context.Context, dueBefore, notBefore time.Time) ([]remind.Due, error) {
	f.dueBefore, f.notBefore = dueBefore, notBefore
	due := f.due
	f.due = nil

	return due, nil
}

// fakeLinks hands out the token "token".
type fakeLinks struct{}

func (fakeLinks) IssueLink(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	return "token", nil
}

// TestASweepSendsTheLink makes sure that every due person gets one SMS with
// the subject and the link, that a long subject is cut, and that the store
// gets the right window of time.
func TestASweepSendsTheLink(t *testing.T) {
	t.Parallel()

	first, second := uuid.NewV7(), uuid.NewV7()

	store := &fakeStore{due: []remind.Due{
		{UserID: uuid.NewV7(), ConversationID: first, PhoneNumber: "+4521650113", Subject: "Lunch"},
		{
			UserID:         uuid.NewV7(),
			ConversationID: second,
			PhoneNumber:    "+4521650114",
			Subject:        "Planning the summer camp of the whole group in Ebeltoft next July, with the tents, the food, the canoes and every parent who drives",
		},
	}}
	messages := &sms.Recorder{}

	service := remind.New(store, fakeLinks{}, messages, "https://chat.example/")

	before := time.Now()

	if err := service.Sweep(t.Context()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	sent := messages.Messages()
	if len(sent) != 2 {
		t.Fatalf("sent %d messages, want 2", len(sent))
	}

	want := "New messages in \"Lunch\" in Chat.\nhttps://chat.example" + address.Conversation(first) + "?t=token"
	if sent[0].Text != want || sent[0].To != "+4521650113" {
		t.Errorf("the first SMS reads %+v, want %q to +4521650113", sent[0], want)
	}

	if !strings.Contains(sent[1].Text, "...\" in Chat.") || !strings.HasSuffix(sent[1].Text, address.Conversation(second)+"?t=token") {
		t.Errorf("the second SMS reads %q, want the subject cut and the link whole", sent[1].Text)
	}

	// The cut drops a space at its end, so the SMS can come out one
	// character short of the limit.
	if got := utf8.RuneCountInString(sent[1].Text); got > 160 || got < 155 {
		t.Errorf("the second SMS holds %d characters, want close to 160 and never more", got)
	}

	if got := before.Sub(store.dueBefore).Round(time.Minute); got != remind.Delay {
		t.Errorf("the store waits %v, want %v", got, remind.Delay)
	}

	if got := before.Sub(store.notBefore).Round(time.Minute); got != remind.MaxAge {
		t.Errorf("the store looks back %v, want %v", got, remind.MaxAge)
	}

	if err := service.Sweep(t.Context()); err != nil || len(messages.Messages()) != 2 {
		t.Errorf("a second sweep sent more, want nothing new")
	}
}
