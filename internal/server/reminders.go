package server

import (
	"context"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/user"
)

// setting stores one switch of the person menu for the person who is signed
// in. The switch sends its value only while it is on, like every checkbox.
// The routes take PUT, which a form on another site cannot send.
func setting(name string, set func(ctx context.Context, id uuid.UUID, on bool) (user.User, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		person, _ := auth.UserFrom(r.Context())

		if _, err := set(r.Context(), person.ID, r.FormValue("on") != ""); err != nil {
			slog.Error("could not store a switch", "switch", name, "error", err)
			http.Error(w, "The switch could not be stored.", http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
