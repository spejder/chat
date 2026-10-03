package push_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/push"
	"github.com/spejder/chat/internal/user"
)

// fakeStore keeps everything in memory. The sending runs on its own, so the
// store guards its fields.
type fakeStore struct {
	mu        sync.Mutex
	keys      push.Keys
	hasKeys   bool
	saved     map[string]uuid.UUID
	targets   []push.Target
	forgotten []string
	counted   []uuid.UUID
}

func (f *fakeStore) Keys(context.Context) (push.Keys, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.keys, f.hasKeys, nil
}

func (f *fakeStore) AddKeys(_ context.Context, keys push.Keys) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.hasKeys {
		f.keys, f.hasKeys = keys, true
	}

	return nil
}

func (f *fakeStore) Save(_ context.Context, userID uuid.UUID, _ []byte, subscription push.Subscription) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.saved == nil {
		f.saved = map[string]uuid.UUID{}
	}

	f.saved[subscription.Endpoint] = userID

	return nil
}

func (f *fakeStore) Remove(context.Context, uuid.UUID, string) error { return nil }

func (f *fakeStore) Forget(_ context.Context, endpoint string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.forgotten = append(f.forgotten, endpoint)

	return nil
}

func (f *fakeStore) Targets(_ context.Context, userIDs []uuid.UUID) ([]push.Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []push.Target

	for _, target := range f.targets {
		if slices.Contains(userIDs, target.UserID) {
			out = append(out, target)
		}
	}

	return out, nil
}

func (f *fakeStore) Unread(_ context.Context, userIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.counted = append(f.counted, userIDs...)

	counts := make(map[uuid.UUID]int, len(userIDs))
	for _, id := range userIDs {
		counts[id] = 3
	}

	return counts, nil
}

// browserKeys makes the two keys that a real browser hands over, so the
// library can encrypt for them.
func browserKeys(t *testing.T) (string, string) {
	t.Helper()

	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("make a browser key: %v", err)
	}

	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("make a browser secret: %v", err)
	}

	encode := base64.RawURLEncoding.EncodeToString

	return encode(private.PublicKey().Bytes()), encode(secret)
}

// target builds a browser of a person at an endpoint.
func target(t *testing.T, userID uuid.UUID, endpoint string) push.Target {
	t.Helper()

	p256dh, secret := browserKeys(t)

	out := push.Target{UserID: userID}
	out.Subscription.Endpoint = endpoint
	out.Subscription.Keys.P256dh = p256dh
	out.Subscription.Keys.Auth = secret

	return out
}

// received is one request that reached the fake push service.
type received struct {
	path    string
	headers http.Header
}

// TestAMessageReachesTheOtherBrowsers sends one message through a fake push
// service and reads what arrived.
func TestAMessageReachesTheOtherBrowsers(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		requests []received
	)

	// The fake push service answers by path: a browser that is still there,
	// one that is gone, and one whose service has a bad day.
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, received{path: r.URL.Path, headers: r.Header.Clone()})
		mu.Unlock()

		switch r.URL.Path {
		case "/gone":
			w.WriteHeader(http.StatusGone)
		case "/broken":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer service.Close()

	ada := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace"}
	grace := user.User{ID: uuid.NewV7(), FullName: "Grace Hopper"}

	store := &fakeStore{targets: []push.Target{
		target(t, ada.ID, service.URL+"/ada"),
		target(t, grace.ID, service.URL+"/grace"),
		target(t, grace.ID, service.URL+"/gone"),
		target(t, grace.ID, service.URL+"/broken"),
	}}

	sender, err := push.New(t.Context(), store, "https://chat.example.com", push.WithHTTPClient(service.Client()))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if sender.PublicKey() == "" || !store.hasKeys {
		t.Fatal("the service made no key pair")
	}

	conversation := chat.Conversation{ID: uuid.NewV7(), Subject: "Lunch"}
	message := chat.Message{AuthorID: ada.ID, AuthorName: ada.FullName, Body: "Are you in?", CreatedAt: time.Now()}

	sender.MessageWritten(t.Context(), conversation, message, []uuid.UUID{grace.ID})
	sender.Wait()

	paths := make([]string, 0, len(requests))
	for _, request := range requests {
		paths = append(paths, request.path)
	}

	slices.Sort(paths)

	if want := []string{"/broken", "/gone", "/grace"}; !slices.Equal(paths, want) {
		t.Fatalf("the push service received %v, want %v", paths, want)
	}

	for _, request := range requests {
		for header, want := range map[string]string{
			"Content-Encoding": "aes128gcm",
			"Ttl":              "86400",
			"Urgency":          "high",
			"Topic":            strings.ReplaceAll(conversation.ID.String(), "-", ""),
		} {
			if got := request.headers.Get(header); got != want {
				t.Errorf("%s: %s = %q, want %q", request.path, header, got, want)
			}
		}

		if !strings.HasPrefix(request.headers.Get("Authorization"), "vapid t=") {
			t.Errorf("%s: the request carries no VAPID signature: %q", request.path, request.headers.Get("Authorization"))
		}
	}

	// The badge counts only the people who hear about the message.
	if want := []uuid.UUID{grace.ID}; !slices.Equal(store.counted, want) {
		t.Errorf("counted the unread messages of %v, want %v", store.counted, want)
	}

	// Only the browser that the service no longer knows goes.
	if want := []string{service.URL + "/gone"}; !slices.Equal(store.forgotten, want) {
		t.Errorf("forgotten = %v, want %v", store.forgotten, want)
	}
}

// TestTheKeyPairIsMadeOnce makes sure that a second start reads the pair of
// the first one.
func TestTheKeyPairIsMadeOnce(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}

	first, err := push.New(t.Context(), store, "http://localhost:8080")
	if err != nil {
		t.Fatalf("first start: %v", err)
	}

	second, err := push.New(t.Context(), store, "http://localhost:8080")
	if err != nil {
		t.Fatalf("second start: %v", err)
	}

	if first.PublicKey() != second.PublicKey() {
		t.Error("the second start made a new pair")
	}
}

// TestSubscribeChecksTheBrowser covers the rules for a subscription.
func TestSubscribeChecksTheBrowser(t *testing.T) {
	t.Parallel()

	ada := user.User{ID: uuid.NewV7(), FullName: "Ada Lovelace"}

	tests := []struct {
		name     string
		endpoint string
		session  []byte
		empty    bool
		ok       bool
	}{
		{name: "a push service", endpoint: "https://push.example.com/abc", session: []byte("s"), ok: true},
		{name: "plain HTTP", endpoint: "http://push.example.com/abc", session: []byte("s")},
		{name: "no address", endpoint: "", session: []byte("s")},
		{name: "no keys", endpoint: "https://push.example.com/abc", session: []byte("s"), empty: true},
		{name: "no session", endpoint: "https://push.example.com/abc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{}

			sender, err := push.New(t.Context(), store, "http://localhost:8080")
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			browser := target(t, ada.ID, test.endpoint).Subscription
			if test.empty {
				browser.Keys.P256dh, browser.Keys.Auth = "", ""
			}

			err = sender.Subscribe(t.Context(), ada, test.session, browser)

			if test.ok && err != nil {
				t.Errorf("subscribe: %v", err)
			}

			if !test.ok && !errors.Is(err, push.ErrBadSubscription) {
				t.Errorf("error = %v, want %v", err, push.ErrBadSubscription)
			}

			if _, stored := store.saved[test.endpoint]; stored != test.ok {
				t.Errorf("stored = %v, want %v", stored, test.ok)
			}
		})
	}
}
