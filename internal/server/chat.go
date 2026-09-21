package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
	"github.com/spejder/chat/internal/web"
)

// Users lists the people that a conversation can reach.
type Users interface {
	List(ctx context.Context) ([]user.User, error)
}

// chatHandlers holds the routes of the conversations.
type chatHandlers struct {
	service *chat.Service
	users   Users
}

// list shows the room beside the sidebar when no conversation is open.
func (h *chatHandlers) list(w http.ResponseWriter, r *http.Request) {
	h.shell(w, r, uuid.Nil(), "All conversations", web.Conversations())
}

// listFragment answers the poll of the sidebar.
func (h *chatHandlers) listFragment(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	summaries, err := h.service.List(r.Context(), person)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	current, err := uuid.Parse(r.URL.Query().Get("current"))
	if err != nil {
		current = uuid.Nil()
	}

	h.render(w, r, web.ConversationList(summaries, current))
}

// shell draws a page inside the sidebar, which every page of a signed in
// person needs.
func (h *chatHandlers) shell(w http.ResponseWriter, r *http.Request, current uuid.UUID, heading string, main templ.Component) {
	person, _ := auth.UserFrom(r.Context())

	summaries, err := h.service.List(r.Context(), person)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	// The page itself arrives as the children of the shell.
	ctx := templ.WithChildren(r.Context(), main)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := web.Shell(summaries, current, heading, sidebarOpen(r)).Render(ctx, w); err != nil {
		slog.Error("could not render the page", "error", err)
	}
}

// newForm shows the form that starts a conversation.
func (h *chatHandlers) newForm(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	others, err := h.others(r.Context(), person)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	h.shell(w, r, uuid.Nil(), "Start a conversation", web.NewConversation(others, "", "", ""))
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
			others, listErr := h.others(r.Context(), person)
			if listErr != nil {
				h.fail(w, r, listErr)

				return
			}

			w.WriteHeader(http.StatusUnprocessableEntity)
			h.shell(w, r, uuid.Nil(), "Start a conversation", web.NewConversation(others, subject, body, message))

			return
		}

		h.fail(w, r, err)

		return
	}

	http.Redirect(w, r, "/conversations/"+conversation.ID.String(), http.StatusSeeOther)
}

// show draws one conversation.
func (h *chatHandlers) show(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	id, ok := conversationID(w, r)
	if !ok {
		return
	}

	conversation, messages, since, err := h.service.Read(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	people, err := h.service.Participants(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return
	}

	page := web.ConversationPage(conversation, people, messages, person, since, messagesVersion(messages))

	h.shell(w, r, conversation.ID, conversation.Subject, page)
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

	version := messagesVersion(messages)
	if r.URL.Query().Get("v") == version {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	h.render(w, r, web.MessageList(conversation, messages, person, readMark(r.URL.Query().Get("since")), version))
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
		if _, readable := readableError(err); readable {
			// The browser keeps the text, because htmx swaps nothing when the
			// answer is not a success.
			http.Error(w, "the message did not go out", http.StatusUnprocessableEntity)

			return
		}

		h.chatError(w, r, err)

		return
	}

	h.render(w, r, web.MessageList(conversation, messages, person, readMark(r.FormValue("since")), messagesVersion(messages)))
}

// conversation reads one conversation for a person who takes part in it.
func (h *chatHandlers) conversation(w http.ResponseWriter, r *http.Request, person user.User, id uuid.UUID) (chat.Conversation, bool) {
	conversation, _, _, err := h.service.Read(r.Context(), person, id)
	if err != nil {
		h.chatError(w, r, err)

		return chat.Conversation{}, false
	}

	return conversation, true
}

// messagesVersion names the state of a conversation. A new message changes
// the count and the newest identifier, and the date belongs in it because the
// date lines read Today and Yesterday.
func messagesVersion(messages []chat.Message) string {
	newest := "none"
	if len(messages) > 0 {
		newest = messages[len(messages)-1].ID.String()
	}

	return fmt.Sprintf("%d-%s-%s", len(messages), newest, time.Now().Local().Format("2006-01-02"))
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

// conversationID reads the identifier out of the path.
func conversationID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)

		return uuid.Nil(), false
	}

	return id, true
}

// readableError turns a rule of internal/chat into a line for the page. The
// second value is false for every other error.
func readableError(err error) (string, bool) {
	switch {
	case errors.Is(err, chat.ErrNoSubject):
		return "Write a subject.", true
	case errors.Is(err, chat.ErrNoParticipants):
		return "Choose at least one other person.", true
	case errors.Is(err, chat.ErrEmptyMessage):
		return "Write a message.", true
	case errors.Is(err, chat.ErrTooLong):
		return "That text is too long.", true
	default:
		return "", false
	}
}

// requireUser sends a visitor without a session to the sign in page.
func requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserFrom(r.Context()); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		next.ServeHTTP(w, r)
	})
}
