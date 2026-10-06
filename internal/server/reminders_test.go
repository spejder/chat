package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"uuid"
)

// sessionFrom returns the session cookie that an answer writes, or nil.
func sessionFrom(answer *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == "chat_session" && cookie.Value != "" {
			return cookie
		}
	}

	return nil
}

// TestALinkSignsInAndOpensTheConversation follows a link from an SMS in a
// browser without a session, in a browser of the same person, and in a
// browser of somebody else.
func TestALinkSignsInAndOpensTheConversation(t *testing.T) {
	t.Parallel()

	built := buildServer(t)

	ada, err := built.users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the writer: %v", err)
	}

	grace, err := built.users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650114")
	if err != nil {
		t.Fatalf("create the reader: %v", err)
	}

	adaSession := signIn(t, built.handler, built.messages, ada)

	started := postAs(t, built.handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)

	path := started.Header().Get("Location")
	conversation := uuid.MustParse(strings.TrimPrefix(path, "/conversations/"))

	token, err := built.auth.IssueLink(t.Context(), grace.ID, conversation)
	if err != nil {
		t.Fatalf("issue the link: %v", err)
	}

	// A browser without a session.
	answer := get(t, built.handler, "/l/"+token, nil)
	if answer.Code != http.StatusSeeOther || answer.Header().Get("Location") != path {
		t.Fatalf("follow: %d to %q, want %d to %q", answer.Code, answer.Header().Get("Location"), http.StatusSeeOther, path)
	}

	if got := answer.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}

	graceSession := sessionFrom(answer)
	if graceSession == nil {
		t.Fatal("the link wrote no session")
	}

	if page := get(t, built.handler, path, graceSession); page.Code != http.StatusOK {
		t.Errorf("the conversation answers %d with the new session, want %d", page.Code, http.StatusOK)
	}

	// The same person again: no new session.
	if again := get(t, built.handler, "/l/"+token, graceSession); sessionFrom(again) != nil {
		t.Error("the link wrote a second session for a browser that is signed in")
	}

	// Somebody else: the link takes over, and the old session ends.
	taken := get(t, built.handler, "/l/"+token, adaSession)
	if sessionFrom(taken) == nil {
		t.Fatal("the link kept the session of the other person")
	}

	if old := get(t, built.handler, "/conversations", adaSession); old.Code != http.StatusSeeOther {
		t.Errorf("the old session answers %d, want it ended", old.Code)
	}

	// An unknown link.
	unknown := get(t, built.handler, "/l/made-up", nil)
	if unknown.Code != http.StatusSeeOther || unknown.Header().Get("Location") != "/login" || sessionFrom(unknown) != nil {
		t.Errorf("an unknown link: %d to %q, want %d to /login", unknown.Code, unknown.Header().Get("Location"), http.StatusSeeOther)
	}
}

// TestTheSMSSwitchIsStored turns the SMS reminders off and on again, and
// makes sure that a visitor cannot.
func TestTheSMSSwitchIsStored(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}

	session := signIn(t, handler, messages, ada)

	put := func(values url.Values, cookie *http.Cookie) int {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/sms-reminders", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		if cookie != nil {
			request.AddCookie(cookie)
		}

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		return recorder.Code
	}

	if code := put(url.Values{}, nil); code != http.StatusSeeOther {
		t.Errorf("a visitor: status = %d, want %d", code, http.StatusSeeOther)
	}

	if code := put(url.Values{}, session); code != http.StatusNoContent {
		t.Fatalf("off: status = %d, want %d", code, http.StatusNoContent)
	}

	if person, _ := users.Get(t.Context(), ada.ID); person.SMSReminders {
		t.Error("the switch is still on")
	}

	if switchOn(t, get(t, handler, "/conversations", session).Body.String()) {
		t.Error("the page shows the switch on, want off")
	}

	if code := put(url.Values{"on": {"true"}}, session); code != http.StatusNoContent {
		t.Fatalf("on: status = %d, want %d", code, http.StatusNoContent)
	}

	if person, _ := users.Get(t.Context(), ada.ID); !person.SMSReminders {
		t.Error("the switch is still off")
	}

	if !switchOn(t, get(t, handler, "/conversations", session).Body.String()) {
		t.Error("the page shows the switch off, want on")
	}
}

// smsSwitch finds the input of the SMS switch, and checkedAttribute the
// bare attribute, not the word in a class such as checked:bg-primary.
var (
	smsSwitch        = regexp.MustCompile(`<input[^>]*data-sms-switch[^>]*>`)
	checkedAttribute = regexp.MustCompile(`\schecked[\s>]`)
)

// switchOn says whether the page renders the SMS switch on.
func switchOn(t *testing.T, page string) bool {
	t.Helper()

	input := smsSwitch.FindString(page)
	if input == "" {
		t.Fatal("the page holds no SMS switch")
	}

	return checkedAttribute.MatchString(input)
}
