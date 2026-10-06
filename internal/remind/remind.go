// Package remind sends an SMS to a person who missed a message, when that
// person has no push notifications. It knows no SQL and no HTTP routes.
//
// A sweep runs every minute. The store decides who is due, and notes the SMS
// in the same step, so one unread stretch of one conversation costs one SMS
// at most. A person who missed messages in several conversations gets one
// SMS for all of them. The SMS names the writers and carries a link that
// signs the person in. No SMS goes out at night to a person who wants quiet
// nights, which is the default.
package remind

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/spejder/chat/internal/address"
	"github.com/spejder/chat/internal/quiet"
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

	// smsLength is the room of one SMS in the GSM alphabet. The subjects get
	// what the rest of the text leaves of it.
	smsLength = 160

	// minSubject is the shortest cut of a subject. A very long address can
	// leave less room, and the SMS then splits in two rather than losing
	// the subject.
	minSubject = 12

	// listedSubjects is how many subjects an SMS about several
	// conversations names. The rest is a count.
	listedSubjects = 2
)

// Due is one person who missed a message in one conversation.
type Due struct {
	UserID         uuid.UUID
	ConversationID uuid.UUID
	PhoneNumber    string
	Subject        string

	// Writers are the full names of the people who wrote what the person
	// missed, the earliest first.
	Writers []string
}

// Store finds the people who are due and notes the SMS for them in the same
// step. A message counts when it was written after notBefore and up to
// dueBefore. In the night it leaves out the people who want quiet nights.
// The rows of one person come together. The store in internal/postgres
// carries it out.
type Store interface {
	Claim(ctx context.Context, dueBefore, notBefore time.Time, night bool) ([]Due, error)
}

// Links makes the link that signs a person in. The nil UUID as the
// conversation makes a link to the list. The service in internal/auth
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

	// now is the clock. A test replaces it with WithClock.
	now func() time.Time
}

// Option changes a service at its start.
type Option func(*Service)

// WithClock replaces the clock, so a test can choose the hour.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// New builds the service. The origin is the address of the site, which the
// link in the SMS starts with.
func New(store Store, links Links, sender sms.Sender, origin string, options ...Option) *Service {
	s := &Service{
		store:  store,
		links:  links,
		sender: sender,
		origin: strings.TrimSuffix(origin, "/"),
		now:    time.Now,
	}

	for _, option := range options {
		option(s)
	}

	return s
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

// Sweep sends one SMS to every person who is due now. In the night of
// internal/quiet it claims only the people who turned the quiet nights off,
// so for everybody else a message that falls due then gets its SMS from the
// first sweep in the morning. The store notes an SMS before it goes out, so a failed send
// costs that SMS and never sends two.
func (s *Service) Sweep(ctx context.Context) error {
	now := s.now()

	due, err := s.store.Claim(ctx, now.Add(-Delay), now.Add(-MaxAge), quiet.Night(now))
	if err != nil {
		return fmt.Errorf("find the people who missed a message: %w", err)
	}

	for _, missed := range byPerson(due) {
		s.send(ctx, missed)
	}

	return nil
}

// send writes one SMS about the conversations that one person missed. One
// conversation gets a link to itself, several get a link to the list.
func (s *Service) send(ctx context.Context, missed []Due) {
	conversation := uuid.Nil()
	target := "/conversations"

	if len(missed) == 1 {
		conversation = missed[0].ConversationID
		target = address.Conversation(conversation)
	}

	token, err := s.links.IssueLink(ctx, missed[0].UserID, conversation)
	if err != nil {
		slog.Error("could not make the link for an SMS reminder", "error", err)

		return
	}

	message := sms.Message{
		To:   missed[0].PhoneNumber,
		Text: text(missed, s.origin+target+"?t="+url.QueryEscape(token)),
	}

	if err := s.sender.Send(ctx, message); err != nil {
		slog.Error("could not send an SMS reminder", "error", err)
	}
}

// byPerson cuts the claimed rows into one group per person. The store
// returns the rows of one person together.
func byPerson(due []Due) [][]Due {
	var out [][]Due

	for i, row := range due {
		if i == 0 || row.UserID != due[i-1].UserID {
			out = append(out, nil)
		}

		out[len(out)-1] = append(out[len(out)-1], row)
	}

	return out
}

// text writes the SMS: who wrote, where, and the link. The writer comes
// first, because that decides whether the person looks now or later. The
// subjects get the room that the rest leaves of one SMS.
//
// Every character stays in the GSM alphabet unless a name or a subject
// brings another one, because one character outside it halves the room of
// an SMS.
func text(missed []Due, link string) string {
	who := writers(missed)

	for limit := smsLength; ; limit-- {
		out := "Chat: " + who + " wrote in " + where(missed, limit) + ".\n" + link

		if utf8.RuneCountInString(out) <= smsLength || limit <= minSubject {
			return out
		}
	}
}

// writers names the people by their first names: "Arne", "Arne and Grace",
// or "Arne and 2 others".
func writers(missed []Due) string {
	var names []string

	seen := map[string]bool{}

	for _, row := range missed {
		for _, name := range row.Writers {
			first := firstName(name)
			if !seen[first] {
				seen[first] = true

				names = append(names, first)
			}
		}
	}

	switch len(names) {
	case 0:
		return "Somebody"
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return names[0] + " and " + strconv.Itoa(len(names)-1) + " others"
	}
}

// where names the conversations: one subject, two subjects, or two and a
// count of the rest. Every subject is cut to limit characters.
func where(missed []Due, limit int) string {
	var quoted []string

	for _, row := range missed[:min(len(missed), listedSubjects)] {
		quoted = append(quoted, `"`+shorten(row.Subject, limit)+`"`)
	}

	if rest := len(missed) - len(quoted); rest > 0 {
		return strings.Join(quoted, ", ") + " and " + strconv.Itoa(rest) + " more"
	}

	return strings.Join(quoted, " and ")
}

// firstName returns the first word of a name.
func firstName(name string) string {
	if fields := strings.Fields(name); len(fields) > 0 {
		return fields[0]
	}

	return name
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
