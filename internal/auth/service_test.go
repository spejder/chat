package auth

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/sms"
	"github.com/spejder/chat/internal/user"
)

// codeInMessage reads the six digits out of the message.
var codeInMessage = regexp.MustCompile(`#(\d{6})`)

// testService builds a service with a clock that the test moves.
func testService(t *testing.T) (*Service, *sms.Recorder, *time.Time, user.User) {
	t.Helper()

	person := user.User{
		ID:          uuid.NewV7(),
		FullName:    "Ada Lovelace",
		Email:       "ada@example.com",
		PhoneNumber: "+4521650113",
	}

	clock := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }

	users := &fakeUsers{people: []user.User{person}}
	messages := &sms.Recorder{}

	service, err := New(users, newFakeStore(users, now), messages, "http://localhost:8080")
	if err != nil {
		t.Fatalf("build the service: %v", err)
	}

	service.now = now

	return service, messages, &clock, person
}

// sendAndRead asks for a code and reads it out of the message.
func sendAndRead(t *testing.T, service *Service, messages *sms.Recorder, email string) string {
	t.Helper()

	started, err := service.Start(t.Context(), email)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if started.Method != MethodCode {
		t.Fatalf("method = %q, want %q", started.Method, MethodCode)
	}

	message, ok := messages.Last()
	if !ok {
		t.Fatal("no message went out")
	}

	found := codeInMessage.FindStringSubmatch(message.Text)
	if found == nil {
		t.Fatalf("the message holds no code: %s", message.Text)
	}

	return found[1]
}

// TestTheMessageBindsTheCodeToTheSite makes sure that the last line has the
// form that a browser needs to fill the code in by itself.
func TestTheMessageBindsTheCodeToTheSite(t *testing.T) {
	t.Parallel()

	service, messages, _, person := testService(t)

	code := sendAndRead(t, service, messages, person.Email)

	message, _ := messages.Last()
	lines := strings.Split(strings.TrimSpace(message.Text), "\n")
	last := lines[len(lines)-1]

	if last != "@localhost #"+code {
		t.Errorf("the last line is %q, want %q", last, "@localhost #"+code)
	}

	if message.To != person.PhoneNumber {
		t.Errorf("the message went to %q, want %q", message.To, person.PhoneNumber)
	}
}

// TestACorrectCodeSignsIn covers the happy path and the session behind it.
func TestACorrectCodeSignsIn(t *testing.T) {
	t.Parallel()

	service, messages, _, person := testService(t)
	code := sendAndRead(t, service, messages, person.Email)

	signedIn, token, err := service.VerifyCode(t.Context(), person.Email, code)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if signedIn.ID != person.ID {
		t.Errorf("signed in %s, want %s", signedIn.ID, person.ID)
	}

	found, ok, err := service.Session(t.Context(), token)
	if err != nil || !ok {
		t.Fatalf("session: ok = %v, error = %v", ok, err)
	}

	if found.ID != person.ID {
		t.Errorf("the session belongs to %s, want %s", found.ID, person.ID)
	}

	if err := service.SignOut(t.Context(), token); err != nil {
		t.Fatalf("sign out: %v", err)
	}

	if _, ok, _ := service.Session(t.Context(), token); ok {
		t.Error("the session still works after the sign out")
	}
}

// TestACodeWorksOnlyOnce makes sure that a used code cannot sign in again.
func TestACodeWorksOnlyOnce(t *testing.T) {
	t.Parallel()

	service, messages, _, person := testService(t)
	code := sendAndRead(t, service, messages, person.Email)

	if _, _, err := service.VerifyCode(t.Context(), person.Email, code); err != nil {
		t.Fatalf("verify: %v", err)
	}

	_, _, err := service.VerifyCode(t.Context(), person.Email, code)
	if !errors.Is(err, ErrNoCode) {
		t.Errorf("error = %v, want %v", err, ErrNoCode)
	}
}

// TestAnOldCodeFails moves the clock past the lifetime.
func TestAnOldCodeFails(t *testing.T) {
	t.Parallel()

	service, messages, clock, person := testService(t)
	code := sendAndRead(t, service, messages, person.Email)

	*clock = clock.Add(CodeLifetime + time.Minute)

	_, _, err := service.VerifyCode(t.Context(), person.Email, code)
	if !errors.Is(err, ErrNoCode) {
		t.Errorf("error = %v, want %v", err, ErrNoCode)
	}
}

// TestGuessingStops makes sure that a code dies after enough wrong tries.
func TestGuessingStops(t *testing.T) {
	t.Parallel()

	service, messages, _, person := testService(t)
	code := sendAndRead(t, service, messages, person.Email)

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}

	for i := 1; i <= MaxCodeAttempts; i++ {
		_, _, err := service.VerifyCode(t.Context(), person.Email, wrong)
		if !errors.Is(err, ErrWrongCode) {
			t.Fatalf("try %d: error = %v, want %v", i, err, ErrWrongCode)
		}
	}

	// The code is dead now, so even the right digits fail.
	_, _, err := service.VerifyCode(t.Context(), person.Email, code)
	if !errors.Is(err, ErrNoCode) {
		t.Errorf("error = %v, want %v", err, ErrNoCode)
	}
}

// TestANewCodeStopsTheOldOne makes sure that only the newest code works.
func TestANewCodeStopsTheOldOne(t *testing.T) {
	t.Parallel()

	service, messages, _, person := testService(t)

	first := sendAndRead(t, service, messages, person.Email)
	second := sendAndRead(t, service, messages, person.Email)

	if first == second {
		t.Skip("the two codes are the same by chance")
	}

	if _, _, err := service.VerifyCode(t.Context(), person.Email, first); !errors.Is(err, ErrWrongCode) {
		t.Errorf("the old code gave %v, want %v", err, ErrWrongCode)
	}

	if _, _, err := service.VerifyCode(t.Context(), person.Email, second); err != nil {
		t.Errorf("the new code gave %v, want no error", err)
	}
}

// TestAnUnknownAddressSendsNothing makes sure that the answer hides whether
// an address exists.
func TestAnUnknownAddressSendsNothing(t *testing.T) {
	t.Parallel()

	service, messages, _, _ := testService(t)

	started, err := service.Start(t.Context(), "nobody@example.com")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	if started.Method != MethodCode {
		t.Errorf("method = %q, want %q", started.Method, MethodCode)
	}

	if len(messages.Messages()) != 0 {
		t.Errorf("a message went out: %+v", messages.Messages())
	}

	if _, _, err := service.VerifyCode(t.Context(), "nobody@example.com", "123456"); !errors.Is(err, ErrNoCode) {
		t.Errorf("error = %v, want %v", err, ErrNoCode)
	}
}

// TestAnUnknownSessionIsNobody covers the middleware path for a stale cookie.
func TestAnUnknownSessionIsNobody(t *testing.T) {
	t.Parallel()

	service, _, _, _ := testService(t)

	if _, ok, err := service.Session(t.Context(), "not-a-token"); ok || err != nil {
		t.Errorf("ok = %v, error = %v, want false and no error", ok, err)
	}

	if _, ok, err := service.Session(t.Context(), ""); ok || err != nil {
		t.Errorf("an empty token gave ok = %v, error = %v", ok, err)
	}
}
