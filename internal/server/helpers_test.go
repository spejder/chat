package server_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/server"
	"github.com/spejder/chat/internal/sms"
	"github.com/spejder/chat/internal/user"
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

	conversations := chat.New(postgres.NewChatStore(pool))

	handler := server.New(server.Config{Auth: service, Chat: conversations, Users: users})

	return handler, messages, users
}

// signIn walks the code path and returns the session cookie of that person.
func signIn(t *testing.T, handler http.Handler, messages *sms.Recorder, person user.User) *http.Cookie {
	t.Helper()

	postForm(t, handler, "/login", url.Values{"email": {person.Email}})

	message, ok := messages.Last()
	if !ok {
		t.Fatal("no message went out")
	}

	found := codeInMessage.FindStringSubmatch(message.Text)
	if found == nil {
		t.Fatalf("the message holds no code: %s", message.Text)
	}

	answer := postForm(t, handler, "/login/code", url.Values{"email": {person.Email}, "code": {found[1]}})

	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == "chat_session" && cookie.Value != "" {
			return cookie
		}
	}

	t.Fatalf("the sign in of %s set no cookie", person.Email)

	return nil
}

// newTestHandler is newHandler for a test that needs no messages and no
// users.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	handler, _, _ := newHandler(t)

	return handler
}
