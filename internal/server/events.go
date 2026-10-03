package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/live"
)

// heartbeat is how often the stream writes a comment while nothing happens.
// A proxy closes a line that stays silent for too long.
const heartbeat = 25 * time.Second

// eventHandlers holds the stream of changes.
type eventHandlers struct {
	hub *live.Hub
}

// stream sends a Server-Sent Event for every change in a conversation of
// this person, until the browser leaves or the server shuts down. The event
// carries only the identifier of the conversation. The page then asks for
// it with the version it holds.
func (h *eventHandlers) stream(w http.ResponseWriter, r *http.Request) {
	person, _ := auth.UserFrom(r.Context())

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	// nginx and others hold an answer back until it ends, unless told not to.
	header.Set("X-Accel-Buffering", "no")

	flusher := http.NewResponseController(w)

	events, cancel := h.hub.Subscribe(person.ID)
	defer cancel()

	// The browser waits three seconds before it connects again after a
	// broken line.
	if _, err := fmt.Fprint(w, "retry: 3000\n\n"); err != nil {
		return
	}

	if err := flusher.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case id, open := <-events:
			if !open {
				return
			}

			if _, err := fmt.Fprintf(w, "event: changed\ndata: %s\n\n", id); err != nil {
				return
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": still here\n\n"); err != nil {
				return
			}
		}

		if err := flusher.Flush(); err != nil {
			return
		}
	}
}
