package server

import (
	"encoding/json"
	"fmt"
	"io"
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
		case event, open := <-events:
			if !open {
				return
			}

			if err := writeEvent(w, event); err != nil {
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

// typingData is what the page reads from a typing event.
type typingData struct {
	Conversation string `json:"conversation"`
	From         string `json:"from"`
	Name         string `json:"name"`
}

// writeEvent writes one event of the stream. A change carries the identifier
// alone, and a typing event carries JSON, which holds no line break, so it
// fits on the one data line that the format allows per field.
func writeEvent(w io.Writer, event live.Event) error {
	switch event.Kind {
	case live.KindTyping:
		data, err := json.Marshal(typingData{
			Conversation: event.Conversation.String(),
			From:         event.From.String(),
			Name:         event.Name,
		})
		if err != nil {
			return fmt.Errorf("write the typing event: %w", err)
		}

		_, err = fmt.Fprintf(w, "event: typing\ndata: %s\n\n", data)

		return err
	default:
		_, err := fmt.Fprintf(w, "event: changed\ndata: %s\n\n", event.Conversation)

		return err
	}
}
