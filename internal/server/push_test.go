package server_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/spejder/chat/internal/push"
)

// sendJSON sends a JSON body as one person.
func sendJSON(t *testing.T, handler http.Handler, method, path, contentType string, body any, session *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("write the body: %v", err)
	}

	request := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(string(data)))
	request.Header.Set("Content-Type", contentType)

	if session != nil {
		request.AddCookie(session)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

// browserSubscription builds what a browser hands over, with real keys, so
// the server can encrypt for it.
func browserSubscription(t *testing.T, endpoint string) push.Subscription {
	t.Helper()

	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("make a browser key: %v", err)
	}

	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("make a browser secret: %v", err)
	}

	var subscription push.Subscription

	subscription.Endpoint = endpoint
	subscription.Keys.P256dh = base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
	subscription.Keys.Auth = base64.RawURLEncoding.EncodeToString(secret)

	return subscription
}

// paths copies the paths that the fake push service received. The service
// writes them on its own goroutine, so the read takes the lock.
func paths(mu *sync.Mutex, received *[]string) []string {
	mu.Lock()
	defer mu.Unlock()

	return slices.Clone(*received)
}

// TestTheSubscriptionRoutesCheckTheirInput covers the answers that do not
// store anything.
func TestTheSubscriptionRoutesCheckTheirInput(t *testing.T) {
	t.Parallel()

	handler, messages, users := newHandler(t)

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the user: %v", err)
	}

	session := signIn(t, handler, messages, ada)
	good := browserSubscription(t, "https://push.example.com/ada")
	plain := browserSubscription(t, "http://push.example.com/ada")

	tests := []struct {
		name        string
		contentType string
		body        any
		session     *http.Cookie
		want        int
	}{
		{name: "a visitor", contentType: "application/json", body: good, want: http.StatusSeeOther},
		{name: "a form", contentType: "application/x-www-form-urlencoded", body: good, session: session, want: http.StatusUnsupportedMediaType},
		{name: "plain HTTP", contentType: "application/json", body: plain, session: session, want: http.StatusUnprocessableEntity},
		{name: "a browser", contentType: "application/json", body: good, session: session, want: http.StatusNoContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer := sendJSON(t, handler, http.MethodPost, "/push/subscriptions", test.contentType, test.body, test.session)

			if answer.Code != test.want {
				t.Errorf("status = %d, want %d\nbody: %s", answer.Code, test.want, answer.Body.String())
			}
		})
	}

	gone := sendJSON(t, handler, http.MethodDelete, "/push/subscriptions", "application/json", map[string]string{"endpoint": good.Endpoint}, session)
	if gone.Code != http.StatusNoContent {
		t.Errorf("unsubscribe: status = %d, want %d", gone.Code, http.StatusNoContent)
	}
}

// TestAMessageReachesTheBrowserOfTheOther walks the whole way: a browser
// subscribes, a message goes out, and the push service receives it. After a
// sign out the browser hears nothing.
func TestAMessageReachesTheBrowserOfTheOther(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		received []string
	)

	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.URL.Path)
		mu.Unlock()

		w.WriteHeader(http.StatusCreated)
	}))
	defer service.Close()

	handler, messages, users, notifications := newHandlerWith(t, push.WithHTTPClient(service.Client()))

	ada, err := users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}

	grace, err := users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the second user: %v", err)
	}

	adaSession := signIn(t, handler, messages, ada)
	graceSession := signIn(t, handler, messages, grace)

	for person, session := range map[string]*http.Cookie{"ada": adaSession, "grace": graceSession} {
		answer := sendJSON(t, handler, http.MethodPost, "/push/subscriptions", "application/json",
			browserSubscription(t, service.URL+"/"+person), session)
		if answer.Code != http.StatusNoContent {
			t.Fatalf("subscribe %s: status = %d", person, answer.Code)
		}
	}

	// The person menu offers the switch with the key of the site.
	page := get(t, handler, "/conversations", graceSession)
	if !strings.Contains(page.Body.String(), `data-push-key="`) {
		t.Error("the page holds no switch for the notifications")
	}

	started := postAs(t, handler, "/conversations", url.Values{
		"subject": {"Lunch"},
		"person":  {grace.ID.String()},
		"body":    {"Are you in?"},
	}, adaSession)
	if started.Code != http.StatusSeeOther {
		t.Fatalf("start: status = %d", started.Code)
	}

	notifications.Wait()

	if got := paths(&mu, &received); len(got) != 1 || got[0] != "/grace" {
		t.Fatalf("the push service received %v, want only /grace", got)
	}

	// After the sign out, the browser of that session hears nothing.
	postAs(t, handler, "/logout", nil, graceSession)

	path := started.Header().Get("Location")
	postAs(t, handler, path+"/messages", url.Values{"body": {"Still there?"}}, adaSession)
	notifications.Wait()

	if got := paths(&mu, &received); len(got) != 1 {
		t.Errorf("the push service received %v after the sign out, want nothing more", got)
	}
}

// TestTheWorkerAndTheManifest makes sure that the two files that must live
// at the root of the site arrive there with the right type, and that the
// browser asks for the worker every time.
func TestTheWorkerAndTheManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path        string
		contentType string
		body        string
	}{
		{path: "/sw.js", contentType: "text/javascript; charset=utf-8", body: `addEventListener("push"`},
		{path: "/manifest.webmanifest", contentType: "application/manifest+json", body: `"start_url"`},
	}

	handler := newTestHandler(t)

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()

			answer := get(t, handler, test.path, nil)

			if answer.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", answer.Code, http.StatusOK)
			}

			if got := answer.Header().Get("Content-Type"); got != test.contentType {
				t.Errorf("Content-Type = %q, want %q", got, test.contentType)
			}

			if got := answer.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
			}

			if !strings.Contains(answer.Body.String(), test.body) {
				t.Errorf("the body misses %q", test.body)
			}
		})
	}
}
