// Package lanehub fans out Lane timeline events over WebSocket and enforces turn locks.
package lanehub

import (
	"encoding/json"
	"sync"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// Client is one browser WebSocket subscriber for a Lane.
type Client struct {
	UserID string
	Send   chan []byte
}

// StatusClient receives cross-lane turn busy updates for the sidebar.
type StatusClient struct {
	UserID string
	Send   chan []byte

	mu    sync.Mutex
	lanes map[string]struct{}
}

// Hub manages per-Lane subscribers and in-memory turn locks.
type Hub struct {
	mu    sync.RWMutex
	lanes map[string]*laneRoom

	statusMu      sync.Mutex
	statusClients map[*StatusClient]struct{}
}

type laneRoom struct {
	clientsMu sync.Mutex
	clients   map[*Client]struct{}

	turnMu sync.Mutex
	busy   bool
}

// New builds an empty hub.
func New() *Hub {
	return &Hub{
		lanes:         make(map[string]*laneRoom),
		statusClients: make(map[*StatusClient]struct{}),
	}
}

func (h *Hub) room(laneID string) *laneRoom {
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.lanes[laneID]
	if !ok {
		r = &laneRoom{clients: make(map[*Client]struct{})}
		h.lanes[laneID] = r
	}

	return r
}

// Subscribe adds a client to a lane room.
func (h *Hub) Subscribe(laneID string, c *Client) {
	r := h.room(laneID)
	r.clientsMu.Lock()
	r.clients[c] = struct{}{}
	r.clientsMu.Unlock()
}

// Unsubscribe removes a client.
func (h *Hub) Unsubscribe(laneID string, c *Client) {
	h.mu.Lock()
	r, ok := h.lanes[laneID]
	h.mu.Unlock()

	if !ok {
		return
	}

	r.clientsMu.Lock()
	if _, exists := r.clients[c]; exists {
		delete(r.clients, c)
		close(c.Send)
	}
	r.clientsMu.Unlock()
}

// SubscribeStatus adds a sidebar turn-status subscriber.
func (h *Hub) SubscribeStatus(c *StatusClient) {
	h.statusMu.Lock()
	h.statusClients[c] = struct{}{}
	h.statusMu.Unlock()
}

// UnsubscribeStatus removes a sidebar turn-status subscriber.
func (h *Hub) UnsubscribeStatus(c *StatusClient) {
	h.statusMu.Lock()
	if _, ok := h.statusClients[c]; ok {
		delete(h.statusClients, c)
		close(c.Send)
	}
	h.statusMu.Unlock()
}

// SetStatusWatches replaces the lane ids a status client cares about.
func (h *Hub) SetStatusWatches(c *StatusClient, laneIDs []string) {
	next := make(map[string]struct{}, len(laneIDs))
	for _, id := range laneIDs {
		if id == "" {
			continue
		}

		next[id] = struct{}{}
	}

	c.mu.Lock()
	c.lanes = next
	c.mu.Unlock()
}

// TurnSnapshot returns busy flags for the given lane ids.
func (h *Hub) TurnSnapshot(laneIDs []string) map[string]bool {
	out := make(map[string]bool, len(laneIDs))
	for _, id := range laneIDs {
		if id == "" {
			continue
		}

		out[id] = h.IsTurnBusy(id)
	}

	return out
}

// DropUser closes all connections for a user on a lane (kick).
func (h *Hub) DropUser(laneID, userID string) {
	h.mu.Lock()
	r, ok := h.lanes[laneID]
	h.mu.Unlock()

	if !ok {
		return
	}

	r.clientsMu.Lock()

	var drop []*Client

	for c := range r.clients {
		if c.UserID == userID {
			drop = append(drop, c)
		}
	}

	for _, c := range drop {
		delete(r.clients, c)
		close(c.Send)
	}
	r.clientsMu.Unlock()
}

// BroadcastJSON sends a JSON payload to all subscribers on a lane.
func (h *Hub) BroadcastJSON(laneID string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}

	h.mu.RLock()
	r, ok := h.lanes[laneID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	r.clientsMu.Lock()
	for c := range r.clients {
		select {
		case c.Send <- data:
		default:
		}
	}
	r.clientsMu.Unlock()
}

// BroadcastMessage fans out a persisted timeline message.
func (h *Hub) BroadcastMessage(laneID string, msg models.LaneMessage) {
	h.BroadcastJSON(laneID, map[string]any{
		"type":    "timeline",
		"message": MessageDTO(msg),
	})
}

// MessageDTO is the JSON shape for timeline events.
func MessageDTO(msg models.LaneMessage) map[string]any {
	return map[string]any{
		"id":              msg.ID,
		"lane_id":         msg.LaneID,
		"sequence_number": msg.SequenceNumber,
		"event_type":      msg.EventType,
		"role":            msg.Role,
		"author_user_id":  msg.AuthorUserID,
		"body":            msg.Body,
		"payload":         msg.Payload,
		"created_at":      msg.CreatedAt,
	}
}

// TryBeginTurn acquires the per-lane turn lock. Returns false if a turn is running.
func (h *Hub) TryBeginTurn(laneID string) bool {
	r := h.room(laneID)
	r.turnMu.Lock()
	if r.busy {
		r.turnMu.Unlock()
		return false
	}

	r.busy = true
	r.turnMu.Unlock()
	h.notifyTurn(laneID, true)

	return true
}

// EndTurn releases the turn lock.
func (h *Hub) EndTurn(laneID string) {
	r := h.room(laneID)
	r.turnMu.Lock()
	wasBusy := r.busy
	r.busy = false
	r.turnMu.Unlock()

	if wasBusy {
		h.notifyTurn(laneID, false)
	}
}

// IsTurnBusy reports whether a turn is in progress.
func (h *Hub) IsTurnBusy(laneID string) bool {
	r := h.room(laneID)
	r.turnMu.Lock()
	defer r.turnMu.Unlock()

	return r.busy
}

func (h *Hub) notifyTurn(laneID string, running bool) {
	data, err := json.Marshal(map[string]any{
		"type":         "turn",
		"lane_id":      laneID,
		"turn_running": running,
	})
	if err != nil {
		return
	}

	h.statusMu.Lock()

	clients := make([]*StatusClient, 0, len(h.statusClients))
	for c := range h.statusClients {
		clients = append(clients, c)
	}
	h.statusMu.Unlock()

	for _, c := range clients {
		c.mu.Lock()
		_, watch := c.lanes[laneID]
		c.mu.Unlock()

		if !watch {
			continue
		}

		select {
		case c.Send <- data:
		default:
		}
	}
}
