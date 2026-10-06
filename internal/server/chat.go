package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"uuid"

	"github.com/spejder/chat/internal/address"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
	"github.com/spejder/chat/internal/web"
)

// Users lists the people that a conversation can reach.
type Users interface {
	List(ctx context.Context) ([]user.User, error)
	SetSMSReminders(ctx context.Context, id uuid.UUID, on bool) (user.User, error)
	SetQuietNights(ctx context.Context, id uuid.UUID, on bool) (user.User, error)
}

// chatHandlers holds the routes of the conversations.
type chatHandlers struct {
	service *chat.Service
	users   Users

	// pushKey is the public half of the push key pair. The page hands it to
	// the browser, which needs it to subscribe.
	pushKey string
}

// list shows the room beside the sidebar when no conversation is open.
func (h *chatHandlers) list(w http.ResponseWriter, r *http.Request) {
	h.shell(w, r, http.StatusOK, web.ShellPage{Title: "Samtaler", OpenOnPhone: true}, web.Conversations())
}

// listFragment answers the sidebar when it asks. It answers 204 when the list
// still stands, so the sidebar does not replace itself for nothing.
func (h *chatHandlers) listFragment(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	summaries, err := h.service.List(r.Context(), person)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	version := listVersion(summaries)
	if r.URL.Query().Get("v") == version {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	current, err := uuid.Parse(r.URL.Query().Get("current"))
	if err != nil {
		current = uuid.Nil()
	}

	h.render(w, r, web.ConversationList(summaries, current, version))
}

// listVersion names the state of the sidebar: which conversations there are,
// how new each of them is, how much of each one this person has not read, and
// whether the others have read the newest message of this person.
func listVersion(summaries []chat.Summary) string {
	var out strings.Builder

	for _, summary := range summaries {
		fmt.Fprintf(&out, "%s:%d:%d:%t;", summary.ID, summary.LastMessageAt.Unix(), summary.Unread, summary.LastRead)
	}

	sum := sha256.Sum256([]byte(out.String()))

	return hex.EncodeToString(sum[:8])
}

// shell draws a page inside the sidebar, which every page of a signed in
// person needs. The handler fills what belongs to its own page, and shell
// adds the sidebar and the people for the dialog that starts a conversation.
func (h *chatHandlers) shell(w http.ResponseWriter, r *http.Request, status int, page web.ShellPage, main templ.Component) {
	person, _ := auth.UserFrom(r.Context())

	summaries, err := h.service.List(r.Context(), person)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	others, err := h.others(r.Context(), person)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	page.Summaries = summaries
	page.Version = listVersion(summaries)
	page.SidebarOpen = sidebarOpen(r)
	page.PushKey = h.pushKey
	page.NewConversation.People = others

	// The page itself arrives as the children of the shell.
	ctx := templ.WithChildren(r.Context(), main)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	if err := web.Shell(page).Render(ctx, w); err != nil {
		slog.Error("could not render the page", "error", err)
	}
}

// newForm shows the blank room with the dialog that starts a conversation
// already open. The pen in the sidebar opens the same dialog without a new
// page, and this address keeps an old link working.
func (h *chatHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	page := web.ShellPage{
		Title:           "Start en samtale",
		NewConversation: web.NewConversationForm{Open: true},
	}

	h.shell(w, r, http.StatusOK, page, web.Conversations())
}

// start opens a conversation and sends the browser into it.
func (h *chatHandlers) start(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	if err := r.ParseForm(); err != nil {
		h.fail(w, r, err)

		return
	}

	subject := r.FormValue("subject")
	body := r.FormValue("body")

	chosen := make([]uuid.UUID, 0, len(r.Form["person"]))

	for _, value := range r.Form["person"] {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			continue
		}

		chosen = append(chosen, id)
	}

	conversation, err := h.service.Start(r.Context(), person, subject, chosen, body)
	if err != nil {
		if message, ok := readableError(err); ok {
			// The dialog comes back open, with everything the reader
			// typed and chose.
			page := web.ShellPage{
				Title: "Start en samtale",
				NewConversation: web.NewConversationForm{
					Subject: subject,
					Body:    body,
					Chosen:  chosen,
					Message: message,
					Open:    true,
				},
			}

			h.shell(w, r, http.StatusUnprocessableEntity, page, web.Conversations())

			return
		}

		h.fail(w, r, err)

		return
	}

	http.Redirect(w, r, address.Conversation(conversation.ID), http.StatusSeeOther)
}

// show draws one conversation.
func (h *chatHandlers) show(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	id, ok := conversationID(w, r)
	if !ok {
		return
	}

	opened, err := h.service.Read(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	people, err := h.service.Participants(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	panel, version, ok := h.panel(w, r, person, id, opened.Since, opened.Messages)
	if !ok {
		return
	}

	panel.People = len(people)

	page := web.ConversationPage(
		opened.Conversation,
		opened.Messages,
		panel,
		version,
		opened.HasOlder,
	)

	h.shell(w, r, http.StatusOK, web.ShellPage{
		Current: opened.Conversation.ID,
		Title:   opened.Conversation.Subject,
		Subject: opened.Conversation.Subject,
		People:  people,
	}, page)
}

// messages answers the poll of a conversation.
//
// The page sends the version it holds. When that version still stands, the
// answer is 204 and htmx swaps nothing, so the page keeps its scrolling, its
// selected text and its work.
func (h *chatHandlers) messages(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	id, ok := conversationID(w, r)
	if !ok {
		return
	}

	conversation, ok := h.conversation(w, r, person, id)
	if !ok {
		return
	}

	messages, err := h.service.Messages(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	panel, version, ok := h.panel(w, r, person, id, readMark(r.URL.Query().Get("since")), messages)
	if !ok {
		return
	}

	if r.URL.Query().Get("v") == version {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	h.render(w, r, web.MessageList(conversation, messages, panel, version))
}

// panel reads what the list needs besides the messages: who takes part and
// when each of them last read the conversation.
func (h *chatHandlers) panel(
	w http.ResponseWriter,
	r *http.Request,
	person user.User,
	id uuid.UUID,
	since time.Time,
	messages []chat.Message,
) (web.Panel, string, bool) {
	readers, err := h.service.Readers(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return web.Panel{}, "", false
	}

	panel := web.Panel{
		Reader:  person,
		Since:   since,
		Readers: readers,
		People:  len(readers),
	}

	return panel, messagesVersion(messages, readers, person), true
}

// write adds a message and answers with the whole list.
func (h *chatHandlers) write(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	id, ok := conversationID(w, r)
	if !ok {
		return
	}

	conversation, ok := h.conversation(w, r, person, id)
	if !ok {
		return
	}

	messages, err := h.service.Write(r.Context(), person, id, r.FormValue("body"))
	if err != nil {
		if message, readable := readableError(err); readable {
			// No body, because htmx swaps whatever comes back and would wipe
			// the conversation. The reason travels in a header instead, and
			// the page writes it under the field.
			trigger, marshalErr := headerJSON(map[string]string{"chat:error": message})
			if marshalErr != nil {
				h.fail(w, r, marshalErr)

				return
			}

			w.Header().Set("HX-Trigger", trigger)
			w.WriteHeader(http.StatusNoContent)

			return
		}

		h.chatError(w, r, err)

		return
	}

	panel, version, ok := h.panel(w, r, person, id, readMark(r.FormValue("since")), messages)
	if !ok {
		return
	}

	// The page empties the write field when it hears this.
	w.Header().Set("HX-Trigger", "chat:sent")

	h.render(w, r, web.MessageList(conversation, messages, panel, version))
}

// conversation reads one conversation for a person who takes part in it.
func (h *chatHandlers) conversation(w http.ResponseWriter, r *http.Request, person user.User, id uuid.UUID) (chat.Conversation, bool) {
	opened, err := h.service.Read(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return chat.Conversation{}, false
	}

	return opened.Conversation, true
}

// typing tells the other people that this person writes right now. The
// page calls it every few seconds while the field holds text.
func (h *chatHandlers) typing(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	id, ok := conversationID(w, r)
	if !ok {
		return
	}

	if err := h.service.Typing(r.Context(), person, id); err != nil {
		h.chatError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// older answers the button above a conversation with the block in front of
// the message it names.
func (h *chatHandlers) older(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	id, ok := conversationID(w, r)
	if !ok {
		return
	}

	before, err := uuid.Parse(r.URL.Query().Get("before"))
	if err != nil {
		http.NotFound(w, r)

		return
	}

	conversation, ok := h.conversation(w, r, person, id)
	if !ok {
		return
	}

	messages, more, err := h.service.Older(r.Context(), person, id, before)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	people, err := h.service.Participants(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	panel := web.Panel{Reader: person, People: len(people), History: true}

	h.render(w, r, web.OlderBlock(conversation, messages, panel, more))
}

// messagesVersion names the state of a conversation. A new message changes
// the count and the newest identifier. The newest reading time belongs in it
// as well, or a message would never gain its Read mark, because the answer
// would stay 204. The date belongs in it because the date lines read Today
// and Yesterday.
func messagesVersion(messages []chat.Message, readers []chat.Reader, reader user.User) string {
	newest := "none"
	if len(messages) > 0 {
		newest = messages[len(messages)-1].ID.String()
	}

	return fmt.Sprintf(
		"%d-%s-%d-%s",
		len(messages),
		newest,
		readersOfNewest(messages, readers, reader),
		time.Now().Local().Format("2006-01-02"),
	)
}

// readersOfNewest counts the other people who have read the newest message of
// this reader. Only that number can change the Read mark, so only that number
// belongs in the version. The plain reading times would not do: every poll
// writes one, and the answer would never be 204 again.
func readersOfNewest(messages []chat.Message, readers []chat.Reader, reader user.User) int {
	var written time.Time

	for _, message := range messages {
		if message.AuthorID == reader.ID {
			written = message.CreatedAt
		}
	}

	if written.IsZero() {
		return 0
	}

	count := 0

	for _, other := range readers {
		if other.ID != reader.ID && other.LastReadAt.After(written) {
			count++
		}
	}

	return count
}

// readMark reads the moment the reader last looked, which the page carries
// from one answer to the next.
func readMark(value string) time.Time {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}

	return time.Unix(seconds, 0)
}

// others lists everybody except the person who is signed in.
func (h *chatHandlers) others(ctx context.Context, person user.User) ([]user.User, error) {
	people, err := h.users.List(ctx)
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(people, func(other user.User) bool {
		return other.ID == person.ID
	}), nil
}

// render writes a component.
func (h *chatHandlers) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("could not render the page", "error", err)
	}
}

// chatError answers a missing conversation with 404. A conversation of other
// people gives the same answer, so the page says nothing about what exists.
func (h *chatHandlers) chatError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, chat.ErrNotFound) {
		http.NotFound(w, r)

		return
	}

	h.fail(w, r, err)
}

// fail answers a fault on our side.
func (h *chatHandlers) fail(w http.ResponseWriter, _ *http.Request, err error) {
	slog.Error("the conversation broke", "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// sidebarOpen reads the cookie that the sidebar writes. A browser that has
// never touched it gets the sidebar open.
func sidebarOpen(r *http.Request) bool {
	cookie, err := r.Cookie("sidebar_state")
	if err != nil {
		return true
	}

	return cookie.Value != "false"
}

// conversationID reads the identifier out of the path, in the short form or
// as a UUID.
func conversationID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := address.ConversationID(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)

		return uuid.Nil(), false
	}

	return id, true
}

// headerJSON writes JSON for a response header. A browser reads the bytes of
// a header as Latin-1, so a Danish letter in UTF-8 would arrive garbled.
// Every character outside ASCII therefore travels as a \u escape, which
// the JSON reader of the page turns back into the letter.
func headerJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("write the header: %w", err)
	}

	var out strings.Builder

	for _, r := range string(data) {
		switch {
		case r < utf8.RuneSelf:
			out.WriteRune(r)
		case r > 0xFFFF:
			high, low := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", high, low)
		default:
			fmt.Fprintf(&out, "\\u%04x", r)
		}
	}

	return out.String(), nil
}

// readableError turns a rule of internal/chat into a line for the page. The
// second value is false for every other error.
func readableError(err error) (string, bool) {
	switch {
	case errors.Is(err, chat.ErrNoSubject):
		return "Skriv et emne.", true
	case errors.Is(err, chat.ErrNoParticipants):
		return "Vælg mindst én anden person.", true
	case errors.Is(err, chat.ErrEmptyMessage):
		return "Skriv en besked.", true
	case errors.Is(err, chat.ErrTooLong):
		return "Teksten er for lang.", true
	default:
		return "", false
	}
}
