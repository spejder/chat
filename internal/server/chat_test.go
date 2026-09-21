package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// versionInPage reads the version of the list out of the page.
var versionInPage = regexp.MustCompile(`/messages\?v=([^"&]+)`)

// get sends a request as one person.
func get(t *testing.T, handler http.Handler, path string, session *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	if session != nil {
		request.AddCookie(session)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

// postAs sends a form as one person.
func postAs(t *testing.T, handler http.Handler, path string, values url.Values, session *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if session != nil {
		request.AddCookie(session)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

// TestAVisitorMustSignIn makes sure that the conversations are closed.
func TestAVisitorMustSignIn(t *testing.T) {
	t.Parallel()

	answer := get(t, newTestHandler(t), "/conversations", nil)

	if answer.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", answer.Code, http.StatusSeeOther)
	}

	if location := answer.Header().Get("Location"); location != "/login" {
		t.Errorf("Location = %q, want %q", location, "/login")
	}
}

// TestAConversationFromStartToAnswer walks the whole path with two people.
func TestAConversationFromStartToAnswer(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	grace, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	adaSession := signIn(t, handler, messages, ada)

	// The form must offer the other person and not the reader.
	form := get(t, handler, "/conversations/new", adaSession)
	if !strings.Contains(form.Body.String(), grace.FullName) {
		t.Errorf("the form does not offer %s", grace.FullName)
	}

	if strings.Contains(form.Body.String(), `value="`+ada.ID.String()+`"`) {
		t.Error("the form offers the reader as a recipient")
	}

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)

	if started.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d\nbody: %s", started.Code, http.StatusSeeOther, started.Body.String())
	}

	path := started.Header().Get("Location")
	if !strings.HasPrefix(path, "/conversations/") {
		t.Fatalf("Location = %q, want a conversation", path)
	}

	// The other person sees the conversation with one unread message.
	graceSession := signIn(t, handler, messages, grace)

	list := get(t, handler, "/conversations", graceSession)
	if !strings.Contains(list.Body.String(), "Lunch") || !strings.Contains(list.Body.String(), "1 new") {
		t.Fatalf("the list does not show the unread conversation: %s", list.Body.String())
	}

	page := get(t, handler, path, graceSession)
	if !strings.Contains(page.Body.String(), "Are you in?") {
		t.Fatalf("the conversation does not show the message: %s", page.Body.String())
	}

	// Reading it clears the count.
	after := get(t, handler, "/conversations", graceSession)
	if strings.Contains(after.Body.String(), "1 new") {
		t.Error("the list still shows an unread message after the read")
	}

	// The answer of the other person reaches the first one.
	written := postAs(t, handler, path+"/messages", url.Values{"body": {"I am in"}}, graceSession)
	if written.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", written.Code, http.StatusOK)
	}

	if !strings.Contains(written.Body.String(), "I am in") {
		t.Errorf("the answer does not hold the new message: %s", written.Body.String())
	}

	poll := get(t, handler, path+"/messages", adaSession)
	if !strings.Contains(poll.Body.String(), "I am in") {
		t.Errorf("the poll of the first person misses the message: %s", poll.Body.String())
	}
}

// TestAStrangerGetsNotFound makes sure that a conversation stays with its
// people.
func TestAStrangerGetsNotFound(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	grace, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	stranger, err := users.Create(t.Context(), "Alan Turing", "alan@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the stranger: %v", err)
	}

	adaSession := signIn(t, handler, messages, ada)

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)

	path := started.Header().Get("Location")

	strangerSession := signIn(t, handler, messages, stranger)

	for _, target := range []string{path, path + "/messages"} {
		answer := get(t, handler, target, strangerSession)
		if answer.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", target, answer.Code, http.StatusNotFound)
		}
	}

	written := postAs(t, handler, path+"/messages", url.Values{"body": {"Hello"}}, strangerSession)
	if written.Code != http.StatusNotFound {
		t.Errorf("writing: status = %d, want %d", written.Code, http.StatusNotFound)
	}

	list := get(t, handler, "/conversations", strangerSession)
	if strings.Contains(list.Body.String(), "Lunch") {
		t.Error("the stranger sees the conversation in the list")
	}
}

// TestAConversationNeedsSomebody makes sure that the form comes back with a
// message instead of an empty conversation.
func TestAConversationNeedsSomebody(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}

	session := signIn(t, handler, messages, ada)

	answer := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"body":    {"Are you in?"},
	}, session)

	if answer.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", answer.Code, http.StatusUnprocessableEntity)
	}

	if !strings.Contains(answer.Body.String(), "Choose at least one other person") {
		t.Errorf("the answer does not say what is missing: %s", answer.Body.String())
	}
}

// TestTheHeaderLinksToTheConversations makes sure that a signed in person
// finds them.
func TestTheHeaderLinksToTheConversations(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	person, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}

	session := signIn(t, handler, messages, person)

	home := get(t, handler, "/", session)
	if !strings.Contains(home.Body.String(), `href="/conversations"`) {
		t.Errorf("the header holds no link to the conversations: %s", home.Body.String())
	}
}

// TestThePollAnswersNothingChanged makes sure that the page keeps what it has
// when the conversation stands still, and receives the list when it moves.
func TestThePollAnswersNothingChanged(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	grace, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	session := signIn(t, handler, messages, ada)

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, session)

	path := started.Header().Get("Location")

	// The page carries the version of the list it holds.
	page := get(t, handler, path, session)

	version := versionInPage.FindStringSubmatch(page.Body.String())
	if version == nil {
		t.Fatalf("the page holds no version: %s", page.Body.String())
	}

	same := get(t, handler, path+"/messages?v="+url.QueryEscape(version[1]), session)
	if same.Code != http.StatusNoContent {
		t.Errorf("the poll with the same version gave %d, want %d", same.Code, http.StatusNoContent)
	}

	if same.Body.Len() != 0 {
		t.Errorf("the poll with the same version carries %d bytes, want none", same.Body.Len())
	}

	old := get(t, handler, path+"/messages?v=0-none-2020-01-01", session)
	if old.Code != http.StatusOK {
		t.Fatalf("the poll with an old version gave %d, want %d", old.Code, http.StatusOK)
	}

	if !strings.Contains(old.Body.String(), "Are you in?") {
		t.Errorf("the answer misses the messages: %s", old.Body.String())
	}
}

// TestAReadChangesTheVersion makes sure that the mark under a message can
// appear at all. A version that ignored the reading would answer 204 forever.
func TestAReadChangesTheVersion(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	grace, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	adaSession := signIn(t, handler, messages, ada)

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)

	path := started.Header().Get("Location")

	page := get(t, handler, path, adaSession)

	found := versionInPage.FindStringSubmatch(page.Body.String())
	if found == nil {
		t.Fatalf("the page holds no version: %s", page.Body.String())
	}

	version := found[1]

	// Nothing has happened, so the poll keeps the page as it is.
	if answer := get(t, handler, path+"/messages?v="+url.QueryEscape(version), adaSession); answer.Code != http.StatusNoContent {
		t.Fatalf("the quiet poll gave %d, want %d", answer.Code, http.StatusNoContent)
	}

	// The other person reads the conversation.
	graceSession := signIn(t, handler, messages, grace)

	if answer := get(t, handler, path, graceSession); answer.Code != http.StatusOK {
		t.Fatalf("the other person could not read the conversation: %d", answer.Code)
	}

	answer := get(t, handler, path+"/messages?v="+url.QueryEscape(version), adaSession)
	if answer.Code != http.StatusOK {
		t.Fatalf("the poll after the reading gave %d, want %d", answer.Code, http.StatusOK)
	}

	if !strings.Contains(answer.Body.String(), "Read") {
		t.Errorf("the answer carries no mark: %s", answer.Body.String())
	}
}
