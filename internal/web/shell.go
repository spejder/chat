package web

import (
	"slices"
	"strings"
	"uuid"

	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/user"
)

// newConversationDialog is the id of the dialog that starts a conversation.
// The button in the sidebar names it, because the dialog sits elsewhere.
const newConversationDialog = "new-conversation"

// ShellPage holds what the shell draws around a page.
type ShellPage struct {
	// Summaries are the lines of the sidebar.
	Summaries []chat.Summary

	// Current is the open conversation, or uuid.Nil when none is open.
	Current uuid.UUID

	// Title is the title of the document.
	Title string

	// Subject and People fill the top bar. An empty subject leaves it
	// blank. The reader is left out of the names.
	Subject string
	People  []user.User

	// SidebarOpen comes from the cookie that the sidebar writes.
	SidebarOpen bool

	// Version is the state of the sidebar list, which the poll sends back.
	Version string

	// NewConversation fills the dialog that starts a conversation.
	NewConversation NewConversationForm
}

// NewConversationForm fills the dialog that starts a conversation.
type NewConversationForm struct {
	// People are the people the reader can choose.
	People []user.User

	// Subject, Body and Chosen keep what the reader typed and chose, when
	// the server refused the form.
	Subject string
	Body    string
	Chosen  []uuid.UUID

	// Message says why the server refused the form.
	Message string

	// Open shows the dialog as soon as the page loads.
	Open bool
}

// chose answers whether the reader chose this person.
func (f NewConversationForm) chose(id uuid.UUID) bool {
	return slices.Contains(f.Chosen, id)
}

// otherNames writes the names of everybody but the reader in one line.
func otherNames(people []user.User, reader uuid.UUID) string {
	out := make([]string, 0, len(people))

	for _, person := range people {
		if person.ID != reader {
			out = append(out, person.FullName)
		}
	}

	return strings.Join(out, ", ")
}
