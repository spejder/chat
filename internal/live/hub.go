// Package live hands the "something changed" events to the open pages of
// this server instance. It knows no SQL and no HTTP routes.
//
// An event carries only the identifier of a conversation. The page answers
// it by asking for that conversation with the version it holds, so the event
// is a doorbell and the rules of what to show stay where they are.
package live

import (
	"sync"
	"uuid"
)

// buffer is how many events a page can fall behind before the hub drops the
// newest ones for it. A dropped event costs nothing lasting, because the next
// event or the slow safety poll of the page brings the same state.
const buffer = 16

// Hub keeps the open pages of this instance, by person.
type Hub struct {
	mu     sync.Mutex
	pages  map[uuid.UUID]map[*page]struct{}
	closed bool
}

// page is one open stream.
type page struct {
	events chan uuid.UUID
}

// New builds an empty hub.
func New() *Hub {
	return &Hub{pages: map[uuid.UUID]map[*page]struct{}{}}
}

// Subscribe opens a stream for one page of a person. The channel closes when
// the page calls cancel or when the hub closes. Call cancel in any case.
func (h *Hub) Subscribe(userID uuid.UUID) (<-chan uuid.UUID, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	p := &page{events: make(chan uuid.UUID, buffer)}

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

// Deliver tells every open page of these people that the conversation
// changed. It never blocks: a page that is behind misses the event.
func (h *Hub) Deliver(conversationID uuid.UUID, people []uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, person := range people {
		for p := range h.pages[person] {
			select {
			case p.events <- conversationID:
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
