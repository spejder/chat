package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/a-h/templ"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/web"
)

// sessionCookie carries the session token.
const sessionCookie = "chat_session"

// authHandlers holds the routes that sign a person in and out.
type authHandlers struct {
	service *auth.Service

	// secureCookies belongs to a site that runs on HTTPS. A browser refuses
	// a secure cookie over plain HTTP, which is how development runs.
	secureCookies bool
}

// page shows the sign in page.
func (h *authHandlers) page(w http.ResponseWriter, r *http.Request) {
	h.renderPanel(w, r, web.EmailPanel("", ""))
}

// start reads the email address and answers with the next panel.
func (h *authHandlers) start(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		h.renderPanel(w, r, web.EmailPanel("", "Type your email address."))

		return
	}

	// The person asked for a code although a passkey exists.
	if r.FormValue("method") == "code" {
		if err := h.service.SendCode(r.Context(), email); err != nil {
			h.fail(w, r, err)

			return
		}

		h.renderPanel(w, r, web.CodePanel(email, ""))

		return
	}

	started, err := h.service.Start(r.Context(), email)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	if started.Method == auth.MethodPasskey {
		options, err := json.Marshal(started.Options)
		if err != nil {
			h.fail(w, r, err)

			return
		}

		h.renderPanel(w, r, web.PasskeyPanel(email, string(options), started.ChallengeID.String()))

		return
	}

	h.renderPanel(w, r, web.CodePanel(email, ""))
}

// code checks the six digits from the message.
func (h *authHandlers) code(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.FormValue("email"))
	code := strings.TrimSpace(r.FormValue("code"))

	person, token, err := h.service.VerifyCode(r.Context(), email, code)
	if err != nil {
		if errors.Is(err, auth.ErrWrongCode) {
			h.renderPanel(w, r, web.CodePanel(email, "That code is wrong. Try again."))

			return
		}

		if errors.Is(err, auth.ErrNoCode) {
			h.renderPanel(w, r, web.EmailPanel(email, "That code is no longer valid. Ask for a new one."))

			return
		}

		h.fail(w, r, err)

		return
	}

	h.setSession(w, token)

	options, challenge, err := h.service.BeginPasskeyRegistration(r.Context(), person)
	if err != nil {
		// The sign in worked, so the person moves on without a passkey.
		slog.Error("could not offer a passkey", "error", err)
		h.renderPanel(w, r, web.SignedIn())

		return
	}

	encoded, err := json.Marshal(options)
	if err != nil {
		h.fail(w, r, err)

		return
	}

	h.renderPanel(w, r, web.PasskeyOffer(string(encoded), challenge.String()))
}

// passkeyLogin finishes a sign in with a passkey. The browser sends JSON, so
// the answer is JSON as well.
func (h *authHandlers) passkeyLogin(w http.ResponseWriter, r *http.Request) {
	challenge, err := uuid.Parse(r.URL.Query().Get("challenge"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "The sign in attempt is unknown. Start again.")

		return
	}

	_, token, err := h.service.FinishPasskeyLogin(r.Context(), challenge, r.Body)
	if err != nil {
		slog.Info("a passkey sign in failed", "error", err)
		writeJSONError(w, http.StatusUnauthorized, "That passkey did not work. Try a code instead.")

		return
	}

	h.setSession(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

// passkeyRegister stores a new passkey for the person who is signed in.
func (h *authHandlers) passkeyRegister(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFrom(r.Context()); !ok {
		writeJSONError(w, http.StatusUnauthorized, "Sign in first.")

		return
	}

	challenge, err := uuid.Parse(r.URL.Query().Get("challenge"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "The attempt is unknown. Start again.")

		return
	}

	if err := h.service.FinishPasskeyRegistration(r.Context(), challenge, r.Body); err != nil {
		slog.Info("a passkey creation failed", "error", err)
		writeJSONError(w, http.StatusBadRequest, "The passkey could not be stored. Try again.")

		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"redirect": "/"})
}

// logout ends the session and sends the person back to the start page.
func (h *authHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if err := h.service.SignOut(r.Context(), cookie.Value); err != nil {
			slog.Error("could not end the session", "error", err)
		}
	}

	h.clearSession(w)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)

		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderPanel answers with the panel alone for htmx, and with the whole page
// for a browser that sends a normal form.
func (h *authHandlers) renderPanel(w http.ResponseWriter, r *http.Request, panel templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	component := panel
	if r.Header.Get("HX-Request") != "true" {
		component = web.Login(panel)
	}

	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("could not render the panel", "error", err)
	}
}

// fail writes the message that a visitor sees when something breaks on our
// side.
func (h *authHandlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("the sign in broke", "error", err)
	h.renderPanel(w, r, web.EmailPanel("", "Something went wrong. Try again."))
}

// setSession writes the cookie that keeps the browser signed in.
func (h *authHandlers) setSession(w http.ResponseWriter, token string) {
	// The Secure flag follows the address of the site, because a browser
	// drops a secure cookie over plain HTTP and development runs that way.
	//nolint:gosec // Secure is true as soon as the origin is https.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(auth.SessionLifetime),
		MaxAge:   int(auth.SessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSession removes the cookie.
func (h *authHandlers) clearSession(w http.ResponseWriter) {
	//nolint:gosec // The flags match the cookie that setSession writes.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// authenticate puts the signed in person into the context of every request.
func (h *authHandlers) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			next.ServeHTTP(w, r)

			return
		}

		person, ok, err := h.service.Session(r.Context(), cookie.Value)
		if err != nil {
			slog.Error("could not read the session", "error", err)
			next.ServeHTTP(w, r)

			return
		}

		if !ok {
			h.clearSession(w)
			next.ServeHTTP(w, r)

			return
		}

		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), person)))
	})
}

// writeJSON answers with JSON.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("could not write the answer", "error", err)
	}
}

// writeJSONError answers with a message that the page can show.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
