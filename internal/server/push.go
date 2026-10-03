package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

	"github.com/spejder/chat/assets"
	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/push"
)

// subscriptionLimit is the largest body that a subscription route reads. A
// real subscription is a few hundred bytes.
const subscriptionLimit = 8 << 10

// pushHandlers holds the routes that a browser uses to subscribe.
type pushHandlers struct {
	service *push.Service
}

// subscribe stores the subscription of this browser.
func (h *pushHandlers) subscribe(w http.ResponseWriter, r *http.Request) {
	var subscription push.Subscription
	if !readJSON(w, r, &subscription) {
		return
	}

	person, _ := auth.UserFrom(r.Context())

	err := h.service.Subscribe(r.Context(), person, auth.SessionFrom(r.Context()), subscription)
	if errors.Is(err, push.ErrBadSubscription) {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)

		return
	}

	if err != nil {
		slog.Error("could not store the subscription", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unsubscribe removes the subscription of this browser.
func (h *pushHandlers) unsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}

	if !readJSON(w, r, &body) {
		return
	}

	person, _ := auth.UserFrom(r.Context())

	if err := h.service.Unsubscribe(r.Context(), person, body.Endpoint); err != nil {
		slog.Error("could not remove the subscription", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// readJSON reads a small JSON body. It accepts only the JSON type, which a
// form on another site cannot send without the consent of this one.
func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "send JSON", http.StatusUnsupportedMediaType)

		return false
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, subscriptionLimit)).Decode(into); err != nil {
		http.Error(w, "the body is no valid JSON", http.StatusBadRequest)

		return false
	}

	return true
}

// serviceWorker hands out the worker that receives the pushes. It lives at
// the root of the site, because a worker controls only the addresses below
// its own. The browser must see a new version at once, so it asks every time
// and gets a short answer when nothing changed.
func serviceWorker(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, "js/sw.js", "text/javascript; charset=utf-8")
}

// manifest hands out the description that lets the site install as an app.
func manifest(w http.ResponseWriter, r *http.Request) {
	serveEmbedded(w, r, "app/manifest.webmanifest", "application/manifest+json")
}

// serveEmbedded sends an embedded file under an address of its own, with a
// fixed type and no cache time.
func serveEmbedded(w http.ResponseWriter, r *http.Request, path, contentType string) {
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Cache-Control", "no-cache")

	if hash, known := assets.Fingerprint(path); known {
		header.Set("ETag", strconv.Quote(hash))
	}

	http.ServeFileFS(w, r, assets.FS, path)
}
