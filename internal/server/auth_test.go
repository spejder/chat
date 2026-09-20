package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// codeInMessage reads the six digits out of the message. The last line binds
// the code to the site, which is where the digits sit.
var codeInMessage = regexp.MustCompile(`#(\d{6})`)

// postForm sends a form the way htmx sends it, so the answer is the panel
// alone.
func postForm(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

// TestSignInWithACode walks the whole path: address, message, code, signed in
// page, sign out.
func TestSignInWithACode(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	person, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}

	answer := postForm(t, handler, "/login", url.Values{"email": {person.Email}})
	if answer.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", answer.Code, http.StatusOK)
	}

	if !strings.Contains(answer.Body.String(), `autocomplete="one-time-code"`) {
		t.Errorf("the answer is not the code panel: %s", answer.Body.String())
	}

	message, ok := messages.Last()
	if !ok {
		t.Fatal("no message went out")
	}

	if message.To != person.PhoneNumber {
		t.Errorf("the message went to %q, want %q", message.To, person.PhoneNumber)
	}

	found := codeInMessage.FindStringSubmatch(message.Text)
	if found == nil {
		t.Fatalf("the message holds no code: %s", message.Text)
	}

	code := found[1]

	answer = postForm(t, handler, "/login/code", url.Values{"email": {person.Email}, "code": {code}})
	if answer.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", answer.Code, http.StatusOK)
	}

	if !strings.Contains(answer.Body.String(), "Create a passkey") {
		t.Errorf("the answer does not offer a passkey: %s", answer.Body.String())
	}

	var session *http.Cookie

	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == "chat_session" {
			session = cookie
		}
	}

	if session == nil || session.Value == "" {
		t.Fatal("the answer sets no session cookie")
	}

	if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
		t.Errorf("the cookie is %+v, want HttpOnly and SameSite=Lax", session)
	}

	// The start page must now greet the person by name.
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	request.AddCookie(session)

	start := httptest.NewRecorder()
	handler.ServeHTTP(start, request)

	if !strings.Contains(start.Body.String(), person.FullName) {
		t.Errorf("the page does not show the name: %s", start.Body.String())
	}

	// Signing out must remove the cookie and the name.
	request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/logout", nil)
	request.AddCookie(session)

	out := httptest.NewRecorder()
	handler.ServeHTTP(out, request)

	if out.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", out.Code, http.StatusSeeOther)
	}

	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	request.AddCookie(session)

	after := httptest.NewRecorder()
	handler.ServeHTTP(after, request)

	if strings.Contains(after.Body.String(), person.FullName) {
		t.Error("the page still shows the name after the sign out")
	}
}

// TestWrongCode makes sure that a wrong code says so and signs nobody in.
func TestWrongCode(t *testing.T) {
	t.Parallel()

	handler, _, users := newHandler(t)

	person, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}

	postForm(t, handler, "/login", url.Values{"email": {person.Email}})

	answer := postForm(t, handler, "/login/code", url.Values{"email": {person.Email}, "code": {"000000"}})

	if !strings.Contains(answer.Body.String(), "That code is wrong") {
		t.Errorf("the answer does not report the wrong code: %s", answer.Body.String())
	}

	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == "chat_session" && cookie.Value != "" {
			t.Error("a wrong code signed the person in")
		}
	}
}

// TestUnknownAddress makes sure that an address without an account sends no
// message and looks the same as one with an account.
func TestUnknownAddress(t *testing.T) {
	t.Parallel()

	handler, messages, _ := newHandler(t)

	answer := postForm(t, handler, "/login", url.Values{"email": {"nobody@example.com"}})

	if !strings.Contains(answer.Body.String(), `autocomplete="one-time-code"`) {
		t.Errorf("the answer is not the code panel: %s", answer.Body.String())
	}

	if len(messages.Messages()) != 0 {
		t.Errorf("a message went out for an unknown address: %+v", messages.Messages())
	}
}

// TestLoginPage makes sure that the page itself renders for a visitor.
func TestLoginPage(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	newTestHandler(t).ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	body := recorder.Body.String()
	for _, want := range []string{`name="email"`, "js/auth.js", "Sign in"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page misses %q", want)
		}
	}
}
