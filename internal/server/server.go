// Package server builds the HTTP routes of the application.
package server

import (
	"net/http"
	"strconv"
	"strings"
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

	return secure(compress(mux))
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
