package postgres_test

import (
	"reflect"
	"testing"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/postgres/postgrestest"
	"github.com/spejder/chat/internal/remind"
	"github.com/spejder/chat/internal/user"
)

// remindStores builds the stores that a reminder test needs, with two
// people in one conversation. Ada writes, and Grace is the reader.
type remindStores struct {
	remind       *postgres.RemindStore
	chat         *postgres.ChatStore
	push         *postgres.PushStore
	auth         *postgres.AuthStore
	users        *postgres.UserStore
	ada, grace   user.User
	conversation uuid.UUID
}

func newRemindStores(t *testing.T) remindStores {
	t.Helper()

	pool := postgrestest.New(t)

	stores := remindStores{
		remind: postgres.NewRemindStore(pool),
		chat:   postgres.NewChatStore(pool),
		push:   postgres.NewPushStore(pool),
		auth:   postgres.NewAuthStore(pool),
		users:  postgres.NewUserStore(pool),
	}

	var err error

	stores.ada, err = stores.users.Create(t.Context(), "Ada Lovelace", "ada@example.com", "+4521650113")
	if err != nil {
		t.Fatalf("create the writer: %v", err)
	}

	stores.grace, err = stores.users.Create(t.Context(), "Grace Hopper", "grace@example.com", "+4521650114")
	if err != nil {
		t.Fatalf("create the reader: %v", err)
	}

	conversation, err := stores.chat.Create(t.Context(), "Lunch", stores.ada.ID,
		[]uuid.UUID{stores.ada.ID, stores.grace.ID}, "Are you in?")
	if err != nil {
		t.Fatalf("start the conversation: %v", err)
	}

	stores.conversation = conversation.ID

	return stores
}

// claim claims as if every message so far were old enough, and young
// enough.
func (s remindStores) claim(t *testing.T) []remind.Due {
	t.Helper()

	now := time.Now()

	due, err := s.remind.Claim(t.Context(), now.Add(time.Minute), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	return due
}

// TestAMissedMessageCostsOneSMS makes sure that a person who missed a
// message is due once, and again only after reading and missing another.
func TestAMissedMessageCostsOneSMS(t *testing.T) {
	t.Parallel()

	stores := newRemindStores(t)

	due := stores.claim(t)
	if len(due) != 1 {
		t.Fatalf("due = %+v, want the reader alone", due)
	}

	want := remind.Due{
		UserID:         stores.grace.ID,
		ConversationID: stores.conversation,
		PhoneNumber:    "+4521650114",
		Subject:        "Lunch",
		Writers:        []string{"Ada Lovelace"},
	}
	if !reflect.DeepEqual(due[0], want) {
		t.Errorf("due = %+v, want %+v", due[0], want)
	}

	if again := stores.claim(t); len(again) != 0 {
		t.Errorf("a second claim = %+v, want nothing before the reader reads", again)
	}

	if _, err := stores.chat.AddMessage(t.Context(), stores.conversation, stores.ada.ID, "Hello?"); err != nil {
		t.Fatalf("write: %v", err)
	}

	if again := stores.claim(t); len(again) != 0 {
		t.Errorf("a claim after one more message = %+v, want nothing in the same stretch", again)
	}

	if _, _, err := stores.chat.MarkRead(t.Context(), stores.conversation, stores.grace.ID); err != nil {
		t.Fatalf("read: %v", err)
	}

	if again := stores.claim(t); len(again) != 0 {
		t.Errorf("a claim after reading = %+v, want nothing", again)
	}

	if _, err := stores.chat.AddMessage(t.Context(), stores.conversation, stores.ada.ID, "Still there?"); err != nil {
		t.Fatalf("write: %v", err)
	}

	if again := stores.claim(t); len(again) != 1 {
		t.Errorf("a claim after a new stretch = %+v, want the reader again", again)
	}
}

// TestAMessageWaitsAndGrowsOld makes sure that a message too young or too
// old causes nothing.
func TestAMessageWaitsAndGrowsOld(t *testing.T) {
	t.Parallel()

	stores := newRemindStores(t)
	now := time.Now()

	young, err := stores.remind.Claim(t.Context(), now.Add(-time.Minute), now.Add(-time.Hour))
	if err != nil || len(young) != 0 {
		t.Errorf("a young message: due = %+v, %v, want nothing", young, err)
	}

	old, err := stores.remind.Claim(t.Context(), now.Add(time.Hour), now.Add(time.Minute))
	if err != nil || len(old) != 0 {
		t.Errorf("an old message: due = %+v, %v, want nothing", old, err)
	}
}

// TestPushAndTheSwitchStopTheSMS makes sure that a person with a live push
// subscription, or with the switch off, or without a number, is never due.
func TestPushAndTheSwitchStopTheSMS(t *testing.T) {
	t.Parallel()

	t.Run("push", func(t *testing.T) {
		t.Parallel()

		stores := newRemindStores(t)

		key := []byte(uuid.NewV7().String())
		if err := stores.auth.SaveSession(t.Context(), key, stores.grace.ID, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("save the session: %v", err)
		}

		if err := stores.push.Save(t.Context(), stores.grace.ID, key, subscription("https://push.example/1")); err != nil {
			t.Fatalf("subscribe: %v", err)
		}

		if due := stores.claim(t); len(due) != 0 {
			t.Errorf("due = %+v, want nobody with push", due)
		}
	})

	t.Run("switch", func(t *testing.T) {
		t.Parallel()

		stores := newRemindStores(t)

		person, err := stores.users.SetSMSReminders(t.Context(), stores.grace.ID, false)
		if err != nil || person.SMSReminders {
			t.Fatalf("turn off: %+v, %v", person, err)
		}

		if due := stores.claim(t); len(due) != 0 {
			t.Errorf("due = %+v, want nobody with the switch off", due)
		}
	})

	t.Run("number", func(t *testing.T) {
		t.Parallel()

		stores := newRemindStores(t)

		if _, err := stores.users.SetPhoneNumber(t.Context(), stores.grace.ID, ""); err != nil {
			t.Fatalf("clear the number: %v", err)
		}

		if due := stores.claim(t); len(due) != 0 {
			t.Errorf("due = %+v, want nobody without a number", due)
		}
	})
}

// TestALinkLivesAndDies makes sure that the store reads a live link and
// ignores an old one.
func TestALinkLivesAndDies(t *testing.T) {
	t.Parallel()

	stores := newRemindStores(t)
	link := auth.Link{UserID: stores.grace.ID, ConversationID: stores.conversation}

	if err := stores.auth.SaveLink(t.Context(), []byte("live"), link, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := stores.auth.SaveLink(t.Context(), []byte("old"), link, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok, err := stores.auth.Link(t.Context(), []byte("live"))
	if err != nil || !ok || got != link {
		t.Errorf("live link = %+v, %v, %v, want %+v", got, ok, err, link)
	}

	if _, ok, err := stores.auth.Link(t.Context(), []byte("old")); err != nil || ok {
		t.Errorf("old link found = %v, %v, want nothing", ok, err)
	}

	// A link to the list belongs to no conversation.
	list := auth.Link{UserID: stores.grace.ID, ConversationID: uuid.Nil()}

	if err := stores.auth.SaveLink(t.Context(), []byte("list"), list, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("save the list link: %v", err)
	}

	got, ok, err = stores.auth.Link(t.Context(), []byte("list"))
	if err != nil || !ok || got != list {
		t.Errorf("list link = %+v, %v, %v, want %+v", got, ok, err, list)
	}
}
