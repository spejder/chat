package server_test

import (
	"net/http"
	"testing"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/server"
	"github.com/spejder/chat/internal/sms"
)

// testOrigin is the address that the tests pretend to run on. A passkey
// belongs to one site, so the value must be a real address.
const testOrigin = "http://localhost:8080"

// newHandler builds the routes with a database and a recorder behind them.
func newHandler(t *testing.T) (http.Handler, *sms.Recorder, *postgres.UserStore) {
	t.Helper()

	pool := postgrestest.New(t)
	users := postgres.NewUserStore(pool)
	messages := &sms.Recorder{}

	service, err := auth.New(users, postgres.NewAuthStore(pool), messages, testOrigin)
	if err != nil {
		t.Fatalf("set up the sign in: %v", err)
	}

	return server.New(server.Config{Auth: service}), messages, users
}

// newTestHandler is newHandler for a test that needs no messages and no
// users.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	handler, _, _ := newHandler(t)

	return handler
}
