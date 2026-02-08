package realtime

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// eventBufferSize is the per-client channel buffer. Events are dropped when full.
const eventBufferSize = 256

// Event represents a data change on a table.
type Event struct {
	Action string         `json:"action"` // "create", "update", "delete"
	Table  string         `json:"table"`
	Record map[string]any `json:"record"`
}

// Hub manages realtime SSE client connections and broadcasts events.
// It is safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client
	nextID  atomic.Uint64
	logger  *slog.Logger
}

// Client represents a connected SSE subscriber.
type Client struct {
	ID     string
	tables map[string]bool
	events chan *Event
}

// Events returns a read-only channel of events for this client.
func (c *Client) Events() <-chan *Event {
	return c.events
}

// NewHub creates a new realtime event hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[string]*Client),
		logger:  logger,
	}
}

// Subscribe creates a new client subscribed to the given tables and registers it.
func (h *Hub) Subscribe(tables map[string]bool) *Client {
	id := fmt.Sprintf("c%d", h.nextID.Add(1))
	client := &Client{
		ID:     id,
		tables: tables,
		events: make(chan *Event, eventBufferSize),
	}

	h.mu.Lock()
	h.clients[id] = client
	h.mu.Unlock()

	h.logger.Debug("client subscribed", "id", id, "tables", tables)
	return client
}

// Unsubscribe removes a client and closes its event channel.
func (h *Hub) Unsubscribe(clientID string) {
	h.mu.Lock()
	client, ok := h.clients[clientID]
	if ok {
		delete(h.clients, clientID)
		close(client.events)
	}
	h.mu.Unlock()

	if ok {
		h.logger.Debug("client unsubscribed", "id", clientID)
	}
}

// Publish sends an event to all clients subscribed to the event's table.
// Uses non-blocking sends — events are dropped for clients with full buffers.
func (h *Hub) Publish(event *Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if !client.tables[event.Table] {
			continue
		}
		select {
		case client.events <- event:
		default:
			h.logger.Warn("client buffer full, dropping event", "clientID", client.ID)
		}
	}
}

// Close disconnects all clients and clears the hub.
// Safe to call multiple times.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, client := range h.clients {
		close(client.events)
		delete(h.clients, id)
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
