// Package live hands the "something changed" events to the open pages of
// this server instance. It knows no SQL and no HTTP routes.
//
// A change carries only the identifier of a conversation. The page answers
// it by asking for that conversation with the version it holds, so the event
// is a doorbell and the rules of what to show stay where they are. A typing
// event also names the person who writes, which the page shows for a few
// seconds and never stores.
package live

import (
	"sync"
	"uuid"
)

// buffer is how many events a page can fall behind before the hub drops the
// newest ones for it. A dropped event costs nothing lasting, because the next
// event or the slow safety poll of the page brings the same state.
const buffer = 16

// The kinds of event.
const (
	// KindChanged says that a conversation changed.
	KindChanged = "changed"

	// KindTyping says that somebody writes in a conversation right now.
	KindTyping = "typing"
)

// Event is one thing that an open page hears about.
type Event struct {
	Kind         string
	Conversation uuid.UUID

	// From and Name name the writer of a typing event.
	From uuid.UUID
	Name string
}

// Hub keeps the open pages of this instance, by person.
type Hub struct {
	mu     sync.Mutex
	pages  map[uuid.UUID]map[*page]struct{}
	closed bool
}

// page is one open stream.
type page struct {
	events chan Event
}

// New builds an empty hub.
func New() *Hub {
	return &Hub{pages: map[uuid.UUID]map[*page]struct{}{}}
}

// Subscribe opens a stream for one page of a person. The channel closes when
// the page calls cancel or when the hub closes. Call cancel in any case.
func (h *Hub) Subscribe(userID uuid.UUID) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	p := &page{events: make(chan Event, buffer)}

	if h.closed {
		close(p.events)

		return p.events, func() {}
	}

	if h.pages[userID] == nil {
		h.pages[userID] = map[*page]struct{}{}
	}

	h.pages[userID][p] = struct{}{}

	var once sync.Once

	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if _, open := h.pages[userID][p]; !open {
				return
			}

			delete(h.pages[userID], p)

			if len(h.pages[userID]) == 0 {
				delete(h.pages, userID)
			}

			close(p.events)
		})
	}

	return p.events, cancel
}

// Deliver hands an event to every open page of these people. It never
// blocks: a page that is behind misses the event.
func (h *Hub) Deliver(event Event, people []uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, person := range people {
		for p := range h.pages[person] {
			select {
			case p.events <- event:
			default:
			}
		}
	}
}

// Close ends every stream. The server calls it when it shuts down, because
// a stream never ends by itself and would hold the shutdown up.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return
	}

	h.closed = true

	for userID, pages := range h.pages {
		for p := range pages {
			close(p.events)
		}

		delete(h.pages, userID)
	}
}
