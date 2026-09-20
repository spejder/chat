package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/spejder/chat/internal/sms"
	"github.com/spejder/chat/internal/user"
)

// The rules of a sign in code.
const (
	// CodeLifetime is how long a code works.
	CodeLifetime = 10 * time.Minute

	// MaxCodeAttempts is how many tries one code allows.
	MaxCodeAttempts = 5

	// ChallengeLifetime is how long a started passkey ceremony may take.
	ChallengeLifetime = 5 * time.Minute

	// SessionLifetime is how long a browser stays signed in.
	SessionLifetime = 30 * 24 * time.Hour
)

var (
	// ErrWrongCode says that the code does not match.
	ErrWrongCode = errors.New("the code is wrong")

	// ErrNoCode says that no live code exists, because none went out, or it
	// is too old, or it is used, or it ran out of tries.
	ErrNoCode = errors.New("the code is no longer valid, ask for a new one")

	// ErrNoChallenge says that the passkey ceremony is unknown or too old.
	ErrNoChallenge = errors.New("the passkey attempt expired, start again")
)

// Method says how a person can sign in.
type Method string

const (
	// MethodPasskey means that the browser must use a passkey.
	MethodPasskey Method = "passkey"

	// MethodCode means that a code went out by SMS.
	MethodCode Method = "code"
)

// Start is the answer to an email address.
type Start struct {
	Method Method

	// Options and ChallengeID carry the passkey request. They are empty for
	// the code method.
	Options     *protocol.CredentialAssertion
	ChallengeID uuid.UUID
}

// Service holds the sign in rules.
type Service struct {
	users  Users
	store  Store
	sender sms.Sender
	web    *webauthn.WebAuthn

	// domain is the host of the site without a port. The last line of the
	// message needs it, so a browser can fill the code in by itself.
	domain string

	// now is the clock. A test replaces it.
	now func() time.Time
}

// New builds the service. The origin is the address of the site, for example
// http://localhost:8080. It sets the identifier that a passkey belongs to.
func New(users Users, store Store, sender sms.Sender, origin string) (*Service, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return nil, fmt.Errorf("read the origin: %w", err)
	}

	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("the origin %q has no host", origin)
	}

	web, err := webauthn.New(&webauthn.Config{
		RPID:          parsed.Hostname(),
		RPDisplayName: "Chat",
		RPOrigins:     []string{strings.TrimSuffix(origin, "/")},
	})
	if err != nil {
		return nil, fmt.Errorf("set up the passkey settings: %w", err)
	}

	return &Service{
		users:  users,
		store:  store,
		sender: sender,
		web:    web,
		domain: parsed.Hostname(),
		now:    time.Now,
	}, nil
}

// Start reads the email address and decides how this person signs in.
//
// An address that belongs to nobody gets the same answer as an address
// without a passkey, and no message goes out. The page must therefore never
// say whether an address exists.
func (s *Service) Start(ctx context.Context, email string) (Start, error) {
	person, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return Start{Method: MethodCode}, nil
		}

		return Start{}, fmt.Errorf("look up the address: %w", err)
	}

	credentials, err := s.credentials(ctx, person.ID)
	if err != nil {
		return Start{}, err
	}

	if len(credentials) == 0 {
		if err := s.sendCode(ctx, person); err != nil {
			return Start{}, err
		}

		return Start{Method: MethodCode}, nil
	}

	options, session, err := s.web.BeginLogin(&webAuthnUser{person: person, credentials: credentials})
	if err != nil {
		return Start{}, fmt.Errorf("start the passkey login: %w", err)
	}

	id, err := s.saveChallenge(ctx, person.ID, PurposeLogin, session)
	if err != nil {
		return Start{}, err
	}

	return Start{Method: MethodPasskey, Options: options, ChallengeID: id}, nil
}

// SendCode sends a code to the person behind an address, even when that
// person has a passkey. An unknown address changes nothing and reports no
// error, so the page never says whether an address exists.
func (s *Service) SendCode(ctx context.Context, email string) error {
	person, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("look up the address: %w", err)
	}

	return s.sendCode(ctx, person)
}

// VerifyCode checks a code and signs the person in.
func (s *Service) VerifyCode(ctx context.Context, email, code string) (user.User, string, error) {
	person, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.User{}, "", ErrNoCode
		}

		return user.User{}, "", fmt.Errorf("look up the address: %w", err)
	}

	stored, ok, err := s.store.LatestCode(ctx, person.ID)
	if err != nil {
		return user.User{}, "", fmt.Errorf("read the code: %w", err)
	}

	if !ok {
		return user.User{}, "", ErrNoCode
	}

	attempts, err := s.store.CountCodeAttempt(ctx, stored.ID)
	if err != nil {
		return user.User{}, "", fmt.Errorf("count the attempt: %w", err)
	}

	if attempts > MaxCodeAttempts {
		if err := s.store.ConsumeCode(ctx, stored.ID); err != nil {
			return user.User{}, "", fmt.Errorf("stop the code: %w", err)
		}

		return user.User{}, "", ErrNoCode
	}

	if subtle.ConstantTimeCompare(stored.Hash, hashCode(person.ID, code)) != 1 {
		if attempts == MaxCodeAttempts {
			if err := s.store.ConsumeCode(ctx, stored.ID); err != nil {
				return user.User{}, "", fmt.Errorf("stop the code: %w", err)
			}
		}

		return user.User{}, "", ErrWrongCode
	}

	if err := s.store.ConsumeCode(ctx, stored.ID); err != nil {
		return user.User{}, "", fmt.Errorf("use up the code: %w", err)
	}

	token, err := s.newSession(ctx, person.ID)
	if err != nil {
		return user.User{}, "", err
	}

	return person, token, nil
}

// FinishPasskeyLogin checks what the authenticator answered and signs the
// person in.
func (s *Service) FinishPasskeyLogin(ctx context.Context, challengeID uuid.UUID, body io.Reader) (user.User, string, error) {
	person, session, err := s.takeChallenge(ctx, challengeID, PurposeLogin)
	if err != nil {
		return user.User{}, "", err
	}

	parsed, err := protocol.ParseCredentialRequestResponseBody(body)
	if err != nil {
		return user.User{}, "", fmt.Errorf("read the passkey answer: %w", err)
	}

	credentials, err := s.credentials(ctx, person.ID)
	if err != nil {
		return user.User{}, "", err
	}

	credential, err := s.web.ValidateLogin(&webAuthnUser{person: person, credentials: credentials}, session, parsed)
	if err != nil {
		return user.User{}, "", fmt.Errorf("check the passkey: %w", err)
	}

	data, err := json.Marshal(credential)
	if err != nil {
		return user.User{}, "", fmt.Errorf("write the passkey: %w", err)
	}

	if err := s.store.UpdateCredential(ctx, credential.ID, data); err != nil {
		return user.User{}, "", fmt.Errorf("store the passkey: %w", err)
	}

	token, err := s.newSession(ctx, person.ID)
	if err != nil {
		return user.User{}, "", err
	}

	return person, token, nil
}

// BeginPasskeyRegistration starts the creation of a passkey for a person who
// is already signed in.
func (s *Service) BeginPasskeyRegistration(ctx context.Context, person user.User) (*protocol.CredentialCreation, uuid.UUID, error) {
	credentials, err := s.credentials(ctx, person.ID)
	if err != nil {
		return nil, uuid.Nil(), err
	}

	options, session, err := s.web.BeginRegistration(
		&webAuthnUser{person: person, credentials: credentials},
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		return nil, uuid.Nil(), fmt.Errorf("start the passkey creation: %w", err)
	}

	id, err := s.saveChallenge(ctx, person.ID, PurposeRegister, session)
	if err != nil {
		return nil, uuid.Nil(), err
	}

	return options, id, nil
}

// FinishPasskeyRegistration stores the new passkey.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, challengeID uuid.UUID, body io.Reader) error {
	person, session, err := s.takeChallenge(ctx, challengeID, PurposeRegister)
	if err != nil {
		return err
	}

	parsed, err := protocol.ParseCredentialCreationResponseBody(body)
	if err != nil {
		return fmt.Errorf("read the passkey answer: %w", err)
	}

	credentials, err := s.credentials(ctx, person.ID)
	if err != nil {
		return err
	}

	credential, err := s.web.CreateCredential(&webAuthnUser{person: person, credentials: credentials}, session, parsed)
	if err != nil {
		return fmt.Errorf("check the new passkey: %w", err)
	}

	data, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("write the passkey: %w", err)
	}

	if err := s.store.AddCredential(ctx, person.ID, credential.ID, data); err != nil {
		return fmt.Errorf("store the passkey: %w", err)
	}

	return nil
}

// Session reads the person behind a cookie value. The second value is false
// when the session is unknown or too old.
func (s *Service) Session(ctx context.Context, token string) (user.User, bool, error) {
	if token == "" {
		return user.User{}, false, nil
	}

	person, ok, err := s.store.SessionUser(ctx, hashToken(token))
	if err != nil {
		return user.User{}, false, fmt.Errorf("read the session: %w", err)
	}

	return person, ok, nil
}

// SignOut ends one session.
func (s *Service) SignOut(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	if err := s.store.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("end the session: %w", err)
	}

	return nil
}

// HasPasskey says whether a person can sign in without a message.
func (s *Service) HasPasskey(ctx context.Context, person user.User) (bool, error) {
	credentials, err := s.credentials(ctx, person.ID)
	if err != nil {
		return false, err
	}

	return len(credentials) > 0, nil
}

// sendCode makes a code, stores its hash and hands the message to the sender.
func (s *Service) sendCode(ctx context.Context, person user.User) error {
	code, err := newCode()
	if err != nil {
		return err
	}

	if err := s.store.ConsumeCodes(ctx, person.ID); err != nil {
		return fmt.Errorf("stop the older codes: %w", err)
	}

	expires := s.now().Add(CodeLifetime)

	if err := s.store.SaveCode(ctx, uuid.NewV7(), person.ID, hashCode(person.ID, code), expires); err != nil {
		return fmt.Errorf("store the code: %w", err)
	}

	message := sms.Message{
		To:   person.PhoneNumber,
		Text: s.codeText(code),
	}

	if err := s.sender.Send(ctx, message); err != nil {
		return fmt.Errorf("send the code: %w", err)
	}

	return nil
}

// codeText writes the message. The last line binds the code to this site, so
// a browser may offer to fill it in.
func (s *Service) codeText(code string) string {
	return fmt.Sprintf(
		"Your code for Chat is %s. It works for %d minutes.\n\n@%s #%s",
		code, int(CodeLifetime.Minutes()), s.domain, code,
	)
}

// credentials reads the passkeys of a person in the shape the library wants.
func (s *Service) credentials(ctx context.Context, userID uuid.UUID) ([]webauthn.Credential, error) {
	stored, err := s.store.Credentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("read the passkeys: %w", err)
	}

	credentials := make([]webauthn.Credential, 0, len(stored))

	for _, row := range stored {
		var credential webauthn.Credential

		if err := json.Unmarshal(row.Data, &credential); err != nil {
			return nil, fmt.Errorf("read a passkey: %w", err)
		}

		credentials = append(credentials, credential)
	}

	return credentials, nil
}

// saveChallenge stores the state of a started ceremony.
func (s *Service) saveChallenge(ctx context.Context, userID uuid.UUID, purpose string, session *webauthn.SessionData) (uuid.UUID, error) {
	data, err := json.Marshal(session)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("write the passkey attempt: %w", err)
	}

	id := uuid.NewV7()

	if err := s.store.SaveChallenge(ctx, id, userID, purpose, data, s.now().Add(ChallengeLifetime)); err != nil {
		return uuid.Nil(), fmt.Errorf("store the passkey attempt: %w", err)
	}

	return id, nil
}

// takeChallenge reads a started ceremony once and removes it.
func (s *Service) takeChallenge(ctx context.Context, id uuid.UUID, purpose string) (user.User, webauthn.SessionData, error) {
	challenge, ok, err := s.store.TakeChallenge(ctx, id, purpose)
	if err != nil {
		return user.User{}, webauthn.SessionData{}, fmt.Errorf("read the passkey attempt: %w", err)
	}

	if !ok {
		return user.User{}, webauthn.SessionData{}, ErrNoChallenge
	}

	var session webauthn.SessionData

	if err := json.Unmarshal(challenge.Data, &session); err != nil {
		return user.User{}, webauthn.SessionData{}, fmt.Errorf("read the passkey attempt: %w", err)
	}

	person, err := s.users.Get(ctx, challenge.UserID)
	if err != nil {
		return user.User{}, webauthn.SessionData{}, fmt.Errorf("read the person: %w", err)
	}

	return person, session, nil
}

// newSession stores a session and returns the value for the cookie.
func (s *Service) newSession(ctx context.Context, userID uuid.UUID) (string, error) {
	token := rand.Text()

	if err := s.store.SaveSession(ctx, hashToken(token), userID, s.now().Add(SessionLifetime)); err != nil {
		return "", fmt.Errorf("store the session: %w", err)
	}

	return token, nil
}

// newCode returns six random digits.
func newCode() (string, error) {
	limit := big.NewInt(1_000_000)

	number, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", fmt.Errorf("make a code: %w", err)
	}

	return fmt.Sprintf("%06d", number), nil
}

// hashCode hashes a code together with the person, so one stolen table gives
// no shortcut from one account to another.
func hashCode(userID uuid.UUID, code string) []byte {
	sum := sha256.Sum256(append(userID[:], code...))

	return sum[:]
}

// hashToken hashes a session token, so the table never holds the value that
// the cookie carries.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))

	return sum[:]
}

// webAuthnUser hands the person to the WebAuthn library in its own shape.
type webAuthnUser struct {
	person      user.User
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte {
	id := u.person.ID

	return id[:]
}

func (u *webAuthnUser) WebAuthnName() string {
	return u.person.Email
}

func (u *webAuthnUser) WebAuthnDisplayName() string {
	return u.person.FullName
}

func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}
