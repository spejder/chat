package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/spejder/chat/internal/address"

	"github.com/SherClockHolmes/webpush-go"

	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
)

const (
	// sendTimeout limits one round of sending. The round runs after the
	// answer to the writer, so it never delays the page.
	sendTimeout = 10 * time.Second

	// timeToLive is how long a push service keeps a message for a browser
	// that is offline. A chat message older than a day is old news.
	timeToLive = 24 * 60 * 60

	// bodyLength is the most characters of a message that a notification
	// shows. The notification is a hint, the conversation is the text.
	bodyLength = 120
)

// Service keeps the subscriptions and sends the notifications.
type Service struct {
	store   Store
	keys    Keys
	subject string
	client  webpush.HTTPClient

	// sending counts the rounds that still run, so a test and the shutdown
	// of the server can wait for them.
	sending sync.WaitGroup
}

// Option changes a detail of the service.
type Option func(*Service)

// WithHTTPClient sends through another client. A test uses it to reach a
// push service that runs on its own certificate.
func WithHTTPClient(client webpush.HTTPClient) Option {
	return func(s *Service) {
		s.client = client
	}
}

// New builds the service. It reads the key pair of the site, and makes one
// the first time. origin is the address of the site, which the push service
// may use to reach whoever runs it.
func New(ctx context.Context, store Store, origin string, options ...Option) (*Service, error) {
	keys, err := loadKeys(ctx, store)
	if err != nil {
		return nil, err
	}

	s := &Service{
		store:   store,
		keys:    keys,
		subject: subject(origin),
		client:  &http.Client{Timeout: sendTimeout},
	}

	for _, option := range options {
		option(s)
	}

	return s, nil
}

// loadKeys reads the key pair, or makes and stores one. The read after the
// write makes two servers that start together use the same pair.
func loadKeys(ctx context.Context, store Store) (Keys, error) {
	keys, ok, err := store.Keys(ctx)
	if err != nil {
		return Keys{}, fmt.Errorf("read the push keys: %w", err)
	}

	if ok {
		return keys, nil
	}

	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return Keys{}, fmt.Errorf("make the push keys: %w", err)
	}

	if err := store.AddKeys(ctx, Keys{Public: public, Private: private}); err != nil {
		return Keys{}, fmt.Errorf("store the push keys: %w", err)
	}

	keys, ok, err = store.Keys(ctx)
	if err != nil {
		return Keys{}, fmt.Errorf("read the push keys: %w", err)
	}

	if !ok {
		return Keys{}, errors.New("read the push keys: the pair is missing after the write")
	}

	slog.Info("made the push keys of the site")

	return keys, nil
}

// subject names who runs the site, which a push service reads to reach
// somebody when the site misbehaves. An HTTPS address is enough. A
// development site on plain HTTP gets a mail address on its host name,
// because the library turns anything else into a mail address anyway.
func subject(origin string) string {
	if strings.HasPrefix(origin, "https://") {
		return origin
	}

	host := "localhost"
	if parsed, err := url.Parse(origin); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}

	return "push@" + host
}

// PublicKey is the half of the key pair that the browser needs to subscribe.
func (s *Service) PublicKey() string {
	if s == nil {
		return ""
	}

	return s.keys.Public
}

// Subscribe stores a browser for a person and the session it signed in with.
func (s *Service) Subscribe(ctx context.Context, person user.User, sessionKey []byte, subscription Subscription) error {
	if err := check(subscription); err != nil {
		return err
	}

	if len(sessionKey) == 0 {
		return fmt.Errorf("%w: no session", ErrBadSubscription)
	}

	if err := s.store.Save(ctx, person.ID, sessionKey, subscription); err != nil {
		return fmt.Errorf("store the subscription: %w", err)
	}

	return nil
}

// Unsubscribe removes a browser of a person.
func (s *Service) Unsubscribe(ctx context.Context, person user.User, endpoint string) error {
	if err := s.store.Remove(ctx, person.ID, endpoint); err != nil {
		return fmt.Errorf("remove the subscription: %w", err)
	}

	return nil
}

// check refuses a subscription that misses a part, or whose endpoint is no
// HTTPS address. The server posts to that address, so it must never point
// into the network of the server over plain HTTP.
func check(subscription Subscription) error {
	endpoint, err := url.Parse(subscription.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
		return fmt.Errorf("%w: the endpoint is no HTTPS address", ErrBadSubscription)
	}

	if len(subscription.Endpoint) > 2048 {
		return fmt.Errorf("%w: the endpoint is too long", ErrBadSubscription)
	}

	keys := subscription.Keys
	if keys.P256dh == "" || keys.Auth == "" || len(keys.P256dh) > 256 || len(keys.Auth) > 256 {
		return fmt.Errorf("%w: the keys are missing or too long", ErrBadSubscription)
	}

	return nil
}

// MessageWritten sends a notification to every browser of the recipients.
// It carries out the Notifier of internal/chat.
//
// The sending runs on its own after the call returns, with a context of its
// own, because the request of the writer ends before the push services
// answer.
func (s *Service) MessageWritten(ctx context.Context, conversation chat.Conversation, message chat.Message, recipients []uuid.UUID) {
	if len(recipients) == 0 {
		return
	}

	// The topic lets a push service drop an older message of the same
	// conversation that still waits for an offline browser. It allows 32
	// characters, which the identifier without its dashes fills exactly.
	topic := strings.ReplaceAll(conversation.ID.String(), "-", "")

	s.sending.Go(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
		defer cancel()

		targets, err := s.store.Targets(ctx, recipients)
		if err != nil {
			slog.Error("could not read the push subscriptions", "error", err)

			return
		}

		if len(targets) == 0 {
			return
		}

		// The count includes the new message, because the round starts
		// after the message is stored. A count that fails to arrive costs
		// only the badge, so the notifications still go out.
		unread, err := s.store.Unread(ctx, recipients)
		if err != nil {
			slog.Error("could not count the unread messages", "error", err)
		}

		for _, target := range targets {
			data, err := json.Marshal(payloadFor(conversation, message, unread[target.UserID]))
			if err != nil {
				slog.Error("could not write the push message", "error", err)

				continue
			}

			s.send(ctx, target, data, topic)
		}
	})
}

// payloadFor writes the message for one person. Everything but the count of
// unread messages is the same for everybody.
func payloadFor(conversation chat.Conversation, message chat.Message, unread int) Payload {
	return Payload{
		Title:  conversation.Subject,
		Body:   message.AuthorName + ": " + shorten(message.Body),
		URL:    address.Conversation(conversation.ID),
		Tag:    "conversation-" + conversation.ID.String(),
		Unread: unread,
	}
}

// Wait blocks until every round of sending is over.
func (s *Service) Wait() {
	s.sending.Wait()
}

// send posts one message to one browser, and forgets the browser when its
// push service no longer knows it.
func (s *Service) send(ctx context.Context, target Target, data []byte, topic string) {
	subscription := &webpush.Subscription{
		Endpoint: target.Subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: target.Subscription.Keys.P256dh,
			Auth:   target.Subscription.Keys.Auth,
		},
	}

	response, err := webpush.SendNotificationWithContext(ctx, data, subscription, &webpush.Options{
		HTTPClient:      s.client,
		Subscriber:      s.subject,
		Topic:           topic,
		TTL:             timeToLive,
		Urgency:         webpush.UrgencyHigh,
		VAPIDPublicKey:  s.keys.Public,
		VAPIDPrivateKey: s.keys.Private,
	})
	if err != nil {
		slog.Error("could not reach the push service", "host", host(target), "error", err)

		return
	}

	defer func() { _ = response.Body.Close() }()

	// The answer carries no content that matters, but reading it lets the
	// client keep the connection.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	switch {
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		if err := s.store.Forget(ctx, target.Subscription.Endpoint); err != nil {
			slog.Error("could not forget the subscription", "error", err)
		}
	case response.StatusCode >= http.StatusBadRequest:
		slog.Error("the push service refused the message", "host", host(target), "status", response.StatusCode)
	}
}

// host names the push service of a target for the log. The whole endpoint
// is a secret of the browser, so the log never holds it.
func host(target Target) string {
	endpoint, err := url.Parse(target.Subscription.Endpoint)
	if err != nil {
		return ""
	}

	return endpoint.Host
}

// shorten cuts a message to the length of a notification and puts it on one
// line.
func shorten(body string) string {
	body = strings.Join(strings.Fields(body), " ")

	if utf8.RuneCountInString(body) <= bodyLength {
		return body
	}

	runes := []rune(body)

	return string(runes[:bodyLength-1]) + "…"
}
