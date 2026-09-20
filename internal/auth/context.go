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
