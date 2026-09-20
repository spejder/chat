// Package server builds the HTTP routes of the application.
package server

import (
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/spejder/chat/assets"
	"github.com/spejder/chat/internal/web"
)

// New returns the handler with every route of the application.
func New() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.StripPrefix("/assets/", static(http.FileServerFS(assets.FS))))
	mux.Handle("GET /{$}", templ.Handler(web.Home()))
	mux.HandleFunc("POST /greet", greet)

	return mux
}

// greet answers the htmx request with the greeting fragment.
func greet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := web.Greeting(time.Now()).Render(r.Context(), w); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// static adds a cache header to the embedded files.
func static(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}
