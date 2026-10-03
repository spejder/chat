package auth

import (
	"context"

	"github.com/spejder/chat/internal/user"
)

// contextKey keeps the user out of reach of other packages.
type contextKey struct{}

// WithUser returns a context that carries the signed in person.
func WithUser(ctx context.Context, signedIn user.User) context.Context {
	return context.WithValue(ctx, contextKey{}, signedIn)
}

// UserFrom returns the signed in person. The second value is false when
// nobody is signed in.
func UserFrom(ctx context.Context) (user.User, bool) {
	signedIn, ok := ctx.Value(contextKey{}).(user.User)

	return signedIn, ok
}

// sessionKey keeps the key of the session out of reach of other packages.
type sessionKey struct{}

// WithSession returns a context that carries the key of the session, which
// is the hash of the token in the cookie. A push subscription belongs to a
// session, so signing out ends it.
func WithSession(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, sessionKey{}, hashToken(token))
}

// SessionFrom returns the key of the session. It is empty when nobody is
// signed in.
func SessionFrom(ctx context.Context) []byte {
	key, _ := ctx.Value(sessionKey{}).([]byte)

	return key
}
