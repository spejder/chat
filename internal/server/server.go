// Package server builds the HTTP routes of the application.
package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/spejder/chat/assets"
	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/chat"
	"github.com/spejder/chat/internal/components"
)

// Config holds what the routes need from the outside.
type Config struct {
	// Auth signs people in and out.
	Auth *auth.Service

	// Chat holds the conversations.
	Chat *chat.Service

	// Users lists the people that a conversation can reach.
	Users Users

	// SecureCookies belongs to a site on HTTPS. A browser drops a secure
	// cookie over plain HTTP, which is how development runs.
	SecureCookies bool
}

// New returns the handler with every route of the application.
func New(config Config) http.Handler {
	handlers := &authHandlers{service: config.Auth, secureCookies: config.SecureCookies}
	conversations := &chatHandlers{service: config.Chat, users: config.Users}

	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.StripPrefix("/assets/", static(http.FileServerFS(assets.FS))))
	mux.Handle("GET /components/{bundle}", components.ScriptsHandler())
	mux.HandleFunc("GET /{$}", start)

	mux.HandleFunc("GET /login", handlers.page)
	mux.HandleFunc("POST /login", handlers.start)
	mux.HandleFunc("POST /login/code", handlers.code)
	mux.HandleFunc("POST /login/passkey", handlers.passkeyLogin)
	mux.HandleFunc("POST /login/passkey/register", handlers.passkeyRegister)
	mux.HandleFunc("POST /logout", handlers.logout)

	// Every conversation route needs a person behind it.
	mux.Handle("GET /conversations", requireUser(http.HandlerFunc(conversations.list)))
	mux.Handle("GET /conversations/list", requireUser(http.HandlerFunc(conversations.listFragment)))
	mux.Handle("GET /conversations/new", requireUser(http.HandlerFunc(conversations.newForm)))
	mux.Handle("POST /conversations", requireUser(http.HandlerFunc(conversations.start)))
	mux.Handle("GET /conversations/{id}", requireUser(http.HandlerFunc(conversations.show)))
	mux.Handle("GET /conversations/{id}/messages", requireUser(http.HandlerFunc(conversations.messages)))
	mux.Handle("GET /conversations/{id}/older", requireUser(http.HandlerFunc(conversations.older)))
	mux.Handle("POST /conversations/{id}/messages", requireUser(http.HandlerFunc(conversations.write)))

	return secure(compress(handlers.authenticate(mux)))
}

// start sends a visitor where they belong. The application has one job, so
// the address / holds no page of its own.
func start(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFrom(r.Context()); ok {
		http.Redirect(w, r, "/conversations", http.StatusSeeOther)

		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// static tells the browser how long it can keep an embedded file.
//
// A request that carries the current hash of the file gets the file for a
// year, because that address can never point at other content. Every other
// request gets a short time and the hash as an ETag, so the next request ends
// in a small answer with the status 304.
func static(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		hash, known := assets.Fingerprint(path)
		switch {
		case known && r.URL.Query().Get("v") == hash:
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "public, max-age=60")
		}

		if known {
			w.Header().Set("ETag", strconv.Quote(hash))
		}

		next.ServeHTTP(w, r)
	})
}
