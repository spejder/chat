package auth

import (
	"context"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/spejder/chat/internal/user"
)

// fakeUsers is a user store in memory.
type fakeUsers struct {
	people []user.User
}

func (f *fakeUsers) Get(_ context.Context, id uuid.UUID) (user.User, error) {
	for _, person := range f.people {
		if person.ID == id {
			return person, nil
		}
	}

	return user.User{}, user.ErrNotFound
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (user.User, error) {
	for _, person := range f.people {
		if equalFold(person.Email, email) {
			return person, nil
		}
	}

	return user.User{}, user.ErrNotFound
}

// equalFold compares two addresses without case, the way the database does.
func equalFold(a, b string) bool {
	return len(a) == len(b) && lower(a) == lower(b)
}

func lower(value string) string {
	out := []byte(value)
	for i, c := range out {
		if c >= 'A' && c <= 'Z' {
			out[i] = c + ('a' - 'A')
		}
	}

	return string(out)
}

// storedCode is one row of the code table.
type storedCode struct {
	id        uuid.UUID
	userID    uuid.UUID
	hash      []byte
	attempts  int
	expiresAt time.Time
	consumed  bool
}

// storedSession is one row of the session table.
type storedSession struct {
	userID    uuid.UUID
	expiresAt time.Time
}

// fakeStore is the Store of this package in memory. It answers with the same
// rules as the Postgres store, including the clock.
type fakeStore struct {
	mu sync.Mutex

	now func() time.Time

	credentials map[uuid.UUID][]Credential
	challenges  map[uuid.UUID]struct {
		challenge Challenge
		purpose   string
		expiresAt time.Time
	}
	codes    []storedCode
	sessions map[string]storedSession
	users    *fakeUsers
}

func newFakeStore(users *fakeUsers, now func() time.Time) *fakeStore {
	return &fakeStore{
		now:         now,
		credentials: map[uuid.UUID][]Credential{},
		challenges: map[uuid.UUID]struct {
			challenge Challenge
			purpose   string
			expiresAt time.Time
		}{},
		sessions: map[string]storedSession{},
		users:    users,
	}
}

func (f *fakeStore) Credentials(_ context.Context, userID uuid.UUID) ([]Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.credentials[userID]), nil
}

func (f *fakeStore) AddCredential(_ context.Context, userID uuid.UUID, id, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.credentials[userID] = append(f.credentials[userID], Credential{ID: id, Data: data})

	return nil
}

func (f *fakeStore) UpdateCredential(_ context.Context, id, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for userID, list := range f.credentials {
		for i, credential := range list {
			if string(credential.ID) == string(id) {
				f.credentials[userID][i].Data = data
			}
		}
	}

	return nil
}

func (f *fakeStore) SaveChallenge(_ context.Context, id, userID uuid.UUID, purpose string, data []byte, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.challenges[id] = struct {
		challenge Challenge
		purpose   string
		expiresAt time.Time
	}{challenge: Challenge{UserID: userID, Data: data}, purpose: purpose, expiresAt: expiresAt}

	return nil
}

func (f *fakeStore) TakeChallenge(_ context.Context, id uuid.UUID, purpose string) (Challenge, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	row, ok := f.challenges[id]
	if !ok || row.purpose != purpose || !row.expiresAt.After(f.now()) {
		return Challenge{}, false, nil
	}

	delete(f.challenges, id)

	return row.challenge, true, nil
}

func (f *fakeStore) ConsumeCodes(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i := range f.codes {
		if f.codes[i].userID == userID {
			f.codes[i].consumed = true
		}
	}

	return nil
}

func (f *fakeStore) SaveCode(_ context.Context, id, userID uuid.UUID, hash []byte, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.codes = append(f.codes, storedCode{id: id, userID: userID, hash: hash, expiresAt: expiresAt})

	return nil
}

func (f *fakeStore) LatestCode(_ context.Context, userID uuid.UUID) (Code, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, row := range slices.Backward(f.codes) {
		if row.userID == userID && !row.consumed && row.expiresAt.After(f.now()) {
			return Code{ID: row.id, Hash: row.hash, Attempts: row.attempts}, true, nil
		}
	}

	return Code{}, false, nil
}

func (f *fakeStore) CountCodeAttempt(_ context.Context, id uuid.UUID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i := range f.codes {
		if f.codes[i].id == id {
			f.codes[i].attempts++

			return f.codes[i].attempts, nil
		}
	}

	return 0, nil
}

func (f *fakeStore) ConsumeCode(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i := range f.codes {
		if f.codes[i].id == id {
			f.codes[i].consumed = true
		}
	}

	return nil
}

func (f *fakeStore) SaveSession(_ context.Context, hash []byte, userID uuid.UUID, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sessions[string(hash)] = storedSession{userID: userID, expiresAt: expiresAt}

	return nil
}

func (f *fakeStore) SessionUser(ctx context.Context, hash []byte) (user.User, bool, error) {
	f.mu.Lock()
	row, ok := f.sessions[string(hash)]
	now := f.now()
	f.mu.Unlock()

	if !ok || !row.expiresAt.After(now) {
		return user.User{}, false, nil
	}

	person, err := f.users.Get(ctx, row.userID)
	if err != nil {
		return user.User{}, false, err
	}

	return person, true, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.sessions, string(hash))

	return nil
}
