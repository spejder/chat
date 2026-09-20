// Package server builds the HTTP routes of the application.
package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/spejder/chat/assets"
	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/components"
	"github.com/spejder/chat/internal/web"
)

// Config holds what the routes need from the outside.
type Config struct {
	// Auth signs people in and out.
	Auth *auth.Service

	// SecureCookies belongs to a site on HTTPS. A browser drops a secure
	// cookie over plain HTTP, which is how development runs.
	SecureCookies bool
}

// New returns the handler with every route of the application.
func New(config Config) http.Handler {
	handlers := &authHandlers{service: config.Auth, secureCookies: config.SecureCookies}

	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.StripPrefix("/assets/", static(http.FileServerFS(assets.FS))))
	mux.Handle("GET /components/{bundle}", components.ScriptsHandler())
	mux.Handle("GET /{$}", templ.Handler(web.Home()))
	mux.HandleFunc("POST /greet", greet)

	mux.HandleFunc("GET /login", handlers.page)
	mux.HandleFunc("POST /login", handlers.start)
	mux.HandleFunc("POST /login/code", handlers.code)
	mux.HandleFunc("POST /login/passkey", handlers.passkeyLogin)
	mux.HandleFunc("POST /login/passkey/register", handlers.passkeyRegister)
	mux.HandleFunc("POST /logout", handlers.logout)

	return secure(compress(handlers.authenticate(mux)))
}

// greet answers the htmx request with the greeting fragment.
func greet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := web.Greeting(time.Now()).Render(r.Context(), w); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
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
