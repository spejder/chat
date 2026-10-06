package server

import (
	"log/slog"
	"net/http"

	"github.com/spejder/chat/internal/auth"
)

// smsReminders stores the switch for the SMS about missed messages. The
// switch sends its value only while it is on, like every checkbox. The
// route takes PUT, which a form on another site cannot send.
func (h *chatHandlers) smsReminders(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	if _, err := h.users.SetSMSReminders(r.Context(), person.ID, r.FormValue("on") != ""); err != nil {
		slog.Error("could not store the switch for the SMS reminders", "error", err)
		http.Error(w, "The switch could not be stored.", http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
