// Package realtime fans events out to connected SSE clients. It is entirely
// in-memory, which is correct because the server is a single process.
package realtime

import (
	"encoding/json"
	"sync"

	"splitfriends/internal/ledger"
)

// Client is one open SSE connection. Messages are pre-encoded SSE frames.
type Client struct {
	UserID string
	Ch     chan []byte
	once   sync.Once
}

// Close makes the client's stream loop exit. Safe to call twice.
func (c *Client) Close() { c.once.Do(func() { close(c.Ch) }) }

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{} // user id -> connections
}

func NewHub() *Hub { return &Hub{clients: map[string]map[*Client]struct{}{}} }

func (h *Hub) Subscribe(userID string) *Client {
	c := &Client{UserID: userID, Ch: make(chan []byte, 64)}
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = map[*Client]struct{}{}
	}
	h.clients[userID][c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	if set := h.clients[c.UserID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.UserID)
		}
	}
	h.mu.Unlock()
	c.Close()
}

// Connected reports whether the user has at least one live stream.
func (h *Hub) Connected(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// Frame encodes an event as an SSE message.
func Frame(ev ledger.Event) []byte {
	data, _ := json.Marshal(ev)
	return []byte("id: " + itoa(ev.ID) + "\nevent: group_event\ndata: " + string(data) + "\n\n")
}

// Publish delivers the event to every connection of every listed user.
// A client whose buffer is full is closed; the browser reconnects with
// Last-Event-ID and replays what it missed, so nothing is lost.
func (h *Hub) Publish(userIDs []string, ev ledger.Event) {
	frame := Frame(ev)
	h.mu.RLock()
	var stale []*Client
	for _, uid := range userIDs {
		for c := range h.clients[uid] {
			select {
			case c.Ch <- frame:
			default:
				stale = append(stale, c)
			}
		}
	}
	h.mu.RUnlock()
	for _, c := range stale {
		h.Unsubscribe(c)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
