// Package remind sends an SMS to a person who missed a message, when that
// person has no push notifications. It knows no SQL and no HTTP routes.
//
// A sweep runs every minute. The store decides who is due, and notes the SMS
// in the same step, so one unread stretch of one conversation costs one SMS
// at most. The SMS carries a link that signs the person in and opens the
// conversation.
package remind

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/sms"
)

const (
	// Delay is how long a message stays unread before an SMS goes out. A
	// person who reads in an open tab or answers quickly never gets one.
	Delay = 15 * time.Minute

	// MaxAge is the oldest message that still causes an SMS. Without it the
	// first sweep after the deploy would send an SMS for every old unread
	// conversation.
	MaxAge = 24 * time.Hour

	// Every is the pause between two sweeps.
	Every = time.Minute

	// smsLength is the room of one SMS in the GSM alphabet. The subject
	// gets what the rest of the text leaves of it.
	smsLength = 160

	// minSubject is the shortest cut of a subject. A very long address can
	// leave less room, and the SMS then splits in two rather than losing
	// the subject.
	minSubject = 12
)

// Due is one person who missed a message in one conversation.
type Due struct {
	UserID         uuid.UUID
	ConversationID uuid.UUID
	PhoneNumber    string
	Subject        string
}

// Store finds the people who are due and notes the SMS for them in the same
// step. A message counts when it was written after notBefore and up to
// dueBefore. The store in internal/postgres carries it out.
type Store interface {
	Claim(ctx context.Context, dueBefore, notBefore time.Time) ([]Due, error)
}

// Links makes the link that signs a person in. The service in internal/auth
// carries it out.
type Links interface {
	IssueLink(ctx context.Context, userID, conversationID uuid.UUID) (string, error)
}

// Service sends the SMS.
type Service struct {
	store  Store
	links  Links
	sender sms.Sender
	origin string

	// now is the clock. A test replaces it.
	now func() time.Time
}

// New builds the service. The origin is the address of the site, which the
// link in the SMS starts with.
func New(store Store, links Links, sender sms.Sender, origin string) *Service {
	return &Service{
		store:  store,
		links:  links,
		sender: sender,
		origin: strings.TrimSuffix(origin, "/"),
		now:    time.Now,
	}
}

// Run sweeps at once and then every minute, until the context ends.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(Every)
	defer ticker.Stop()

	for {
		if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			slog.Error("could not send the SMS reminders", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Sweep sends an SMS to every person who is due now. The store notes an SMS
// before it goes out, so a failed send costs that SMS and never sends two.
func (s *Service) Sweep(ctx context.Context) error {
	now := s.now()

	due, err := s.store.Claim(ctx, now.Add(-Delay), now.Add(-MaxAge))
	if err != nil {
		return fmt.Errorf("find the people who missed a message: %w", err)
	}

	for _, person := range due {
		token, err := s.links.IssueLink(ctx, person.UserID, person.ConversationID)
		if err != nil {
			slog.Error("could not make the link for an SMS reminder", "error", err)

			continue
		}

		message := sms.Message{
			To:   person.PhoneNumber,
			Text: s.text(person.Subject, person.ConversationID, token),
		}

		if err := s.sender.Send(ctx, message); err != nil {
			slog.Error("could not send an SMS reminder", "error", err)
		}
	}

	return nil
}

// text writes the SMS. The link is the address of the conversation, which
// works for as long as the conversation exists. The token in it signs the
// person in during its first hours.
//
// Every character stays in the GSM alphabet unless the subject brings
// another one, because one character outside it halves the room of an SMS.
func (s *Service) text(subject string, conversationID uuid.UUID, token string) string {
	const format = "New messages in \"%s\" in Chat.\n%s"

	link := s.origin + "/conversations/" + conversationID.String() + "?t=" + url.QueryEscape(token)
	room := smsLength - len(fmt.Sprintf(format, "", link))

	return fmt.Sprintf(format, shorten(subject, max(room, minSubject)), link)
}

// shorten cuts a subject to at most limit characters and marks the cut with
// three dots.
func shorten(subject string, limit int) string {
	runes := []rune(strings.TrimSpace(subject))
	if len(runes) <= limit {
		return string(runes)
	}

	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}
