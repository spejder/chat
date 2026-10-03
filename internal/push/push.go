// Package push sends a notification to the browsers of the people in a
// conversation when a message arrives. It knows no SQL and no HTTP routes.
//
// A browser subscribes with the public VAPID key of the site. The push
// service of that browser, for example the one of Google or Mozilla, hands
// out an endpoint. The server posts an encrypted message to that endpoint,
// and the push service wakes the service worker of the site in the browser.
package push

import (
	"context"
	"errors"
	"uuid"
)

// ErrBadSubscription says that a subscription misses a part or points
// somewhere that is no push service.
var ErrBadSubscription = errors.New("the subscription is not valid")

// Keys is the VAPID key pair of the site, both halves in base64url.
type Keys struct {
	Public  string
	Private string
}

// Subscription is one browser that wants to hear about new messages. The
// fields carry the names of PushSubscription.toJSON() in the browser.
type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Target is a subscription together with the person it belongs to.
type Target struct {
	UserID       uuid.UUID
	Subscription Subscription
}

// Store keeps the key pair and the subscriptions. The store in
// internal/postgres carries it out.
type Store interface {
	// Keys reads the key pair. The second value is false when the site has
	// none yet.
	Keys(ctx context.Context) (Keys, bool, error)

	// AddKeys stores a pair unless one is there already.
	AddKeys(ctx context.Context, keys Keys) error

	// Save stores a subscription for a person and a session. A known
	// endpoint moves to that person and session.
	Save(ctx context.Context, userID uuid.UUID, sessionKey []byte, subscription Subscription) error

	// Remove deletes one subscription of a person.
	Remove(ctx context.Context, userID uuid.UUID, endpoint string) error

	// Forget deletes a subscription that the push service no longer knows.
	Forget(ctx context.Context, endpoint string) error

	// Targets reads the live subscriptions of some people.
	Targets(ctx context.Context, userIDs []uuid.UUID) ([]Target, error)
}

// Payload is what the service worker receives. It shows title and body, the
// tag makes a newer notification of the same conversation replace an older
// one, and a click opens the address.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}
