package remind_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/spejder/chat/internal/address"
	"github.com/spejder/chat/internal/quiet"
	"github.com/spejder/chat/internal/remind"
	"github.com/spejder/chat/internal/sms"
)

// fakeStore hands out a fixed list once and notes the times it got.
type fakeStore struct {
	due                  []remind.Due
	claimed              bool
	dueBefore, notBefore time.Time
}

func (f *fakeStore) Claim(_ context.Context, dueBefore, notBefore time.Time) ([]remind.Due, error) {
	f.claimed = true
	f.dueBefore, f.notBefore = dueBefore, notBefore
	due := f.due
	f.due = nil

	return due, nil
}

// fakeLinks hands out the token "token" and notes the conversations.
type fakeLinks struct {
	conversations []uuid.UUID
}

func (f *fakeLinks) IssueLink(_ context.Context, _, conversationID uuid.UUID) (string, error) {
	f.conversations = append(f.conversations, conversationID)

	return "token", nil
}

// at returns a clock that stands still at an hour of a day in Copenhagen.
func at(t *testing.T, hour int) func() time.Time {
	t.Helper()

	moment := time.Date(2026, time.October, 6, hour, 30, 0, 0, quiet.Location())

	return func() time.Time { return moment }
}

// sweep runs one sweep at an hour and returns what went out.
func sweep(t *testing.T, hour int, due []remind.Due) ([]sms.Message, *fakeStore, *fakeLinks) {
	t.Helper()

	store := &fakeStore{due: due}
	links := &fakeLinks{}
	messages := &sms.Recorder{}

	service := remind.New(store, links, messages, "https://chat.example/", remind.WithClock(at(t, hour)))

	if err := service.Sweep(t.Context()); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	return messages.Messages(), store, links
}

// TestAnSMSNamesTheWriter makes sure that one missed conversation gives an
// SMS with the first names of the writers, the subject and a link to the
// conversation, and that the store gets the right window of time.
func TestAnSMSNamesTheWriter(t *testing.T) {
	t.Parallel()

	ada, grace := uuid.NewV7(), uuid.NewV7()
	lunch, camp := uuid.NewV7(), uuid.NewV7()

	sent, store, links := sweep(t, 12, []remind.Due{
		{UserID: ada, ConversationID: lunch, PhoneNumber: "+4521650113", Subject: "Lunch", Writers: []string{"Arne Jørgensen"}},
		{UserID: grace, ConversationID: camp, PhoneNumber: "+4521650114", Subject: "Camp", Writers: []string{
			"Arne Jørgensen", "Grace Hopper", "Alan Turing",
		}},
	})

	if len(sent) != 2 {
		t.Fatalf("sent %d messages, want 2", len(sent))
	}

	want := "Chat: Arne wrote in \"Lunch\".\nhttps://chat.example" + address.Conversation(lunch) + "?t=token"
	if sent[0].Text != want || sent[0].To != "+4521650113" {
		t.Errorf("the first SMS reads %+v, want %q to +4521650113", sent[0], want)
	}

	if !strings.HasPrefix(sent[1].Text, "Chat: Arne and 2 others wrote in \"Camp\".") {
		t.Errorf("the second SMS reads %q, want the first writer and a count", sent[1].Text)
	}

	if links.conversations[0] != lunch || links.conversations[1] != camp {
		t.Errorf("the links belong to %v, want the two conversations", links.conversations)
	}

	now := at(t, 12)()

	if got := now.Sub(store.dueBefore); got != remind.Delay {
		t.Errorf("the store waits %v, want %v", got, remind.Delay)
	}

	if got := now.Sub(store.notBefore); got != remind.MaxAge {
		t.Errorf("the store looks back %v, want %v", got, remind.MaxAge)
	}
}

// TestSeveralConversationsMakeOneSMS makes sure that a person who missed
// three conversations gets one SMS with two subjects, a count of the rest,
// and a link to the list.
func TestSeveralConversationsMakeOneSMS(t *testing.T) {
	t.Parallel()

	ada := uuid.NewV7()

	sent, _, links := sweep(t, 12, []remind.Due{
		{UserID: ada, ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: "Lunch", Writers: []string{"Arne Jørgensen"}},
		{UserID: ada, ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: "Camp", Writers: []string{"Grace Hopper", "Arne Jørgensen"}},
		{UserID: ada, ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: "Canoes", Writers: []string{"Arne Jørgensen"}},
	})

	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}

	want := "Chat: Arne and Grace wrote in \"Lunch\", \"Camp\" and 1 more.\nhttps://chat.example/conversations?t=token"
	if sent[0].Text != want {
		t.Errorf("the SMS reads %q, want %q", sent[0].Text, want)
	}

	if len(links.conversations) != 1 || links.conversations[0] != uuid.Nil() {
		t.Errorf("the links belong to %v, want one link to the list", links.conversations)
	}
}

// TestLongSubjectsFitInOneSMS makes sure that the subjects shrink until the
// SMS fits in 160 characters.
func TestLongSubjectsFitInOneSMS(t *testing.T) {
	t.Parallel()

	ada := uuid.NewV7()
	long := "Planning the summer camp of the whole group in Ebeltoft next July, with the tents and the canoes"

	for _, due := range [][]remind.Due{
		{{UserID: ada, ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: long, Writers: []string{"Arne Jørgensen"}}},
		{
			{UserID: ada, ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: long, Writers: []string{"Arne Jørgensen"}},
			{UserID: ada, ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: long, Writers: []string{"Grace Hopper"}},
		},
	} {
		sent, _, _ := sweep(t, 12, due)

		if len(sent) != 1 {
			t.Fatalf("sent %d messages, want 1", len(sent))
		}

		// The cut drops a space at its end, so the SMS can come out a little
		// short of the limit.
		if got := utf8.RuneCountInString(sent[0].Text); got > 160 || got < 150 {
			t.Errorf("the SMS holds %d characters, want close to 160 and never more: %q", got, sent[0].Text)
		}

		if !strings.Contains(sent[0].Text, "...") {
			t.Errorf("the SMS reads %q, want the subject cut", sent[0].Text)
		}
	}
}

// TestTheNightIsQuiet makes sure that a sweep in the quiet hours claims
// nothing, so the SMS waits for the morning.
func TestTheNightIsQuiet(t *testing.T) {
	t.Parallel()

	due := func() []remind.Due {
		return []remind.Due{{UserID: uuid.NewV7(), ConversationID: uuid.NewV7(), PhoneNumber: "+4521650113", Subject: "Lunch"}}
	}

	for _, hour := range []int{quiet.From, 23, 0, 3, quiet.Until - 1} {
		sent, store, _ := sweep(t, hour, due())
		if len(sent) != 0 || store.claimed {
			t.Errorf("at %d:30 the sweep claimed %v and sent %d, want nothing", hour, store.claimed, len(sent))
		}
	}

	for _, hour := range []int{quiet.Until, 12, quiet.From - 1} {
		if sent, _, _ := sweep(t, hour, due()); len(sent) != 1 {
			t.Errorf("at %d:30 the sweep sent %d, want 1", hour, len(sent))
		}
	}
}
