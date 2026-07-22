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

// Hub manages per-Lane subscribers and in-memory turn locks.
type Hub struct {
	mu    sync.RWMutex
	lanes map[string]*laneRoom
}

type laneRoom struct {
	clientsMu sync.Mutex
	clients   map[*Client]struct{}

	turnMu sync.Mutex
	busy   bool
}

// New builds an empty hub.
func New() *Hub {
	return &Hub{lanes: make(map[string]*laneRoom)}
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
		"payload":         json.RawMessage(msg.Payload),
		"created_at":      msg.CreatedAt,
	}
}

// TryBeginTurn acquires the per-lane turn lock. Returns false if a turn is running.
func (h *Hub) TryBeginTurn(laneID string) bool {
	r := h.room(laneID)
	r.turnMu.Lock()
	defer r.turnMu.Unlock()
	if r.busy {
		return false
	}
	r.busy = true
	return true
}

// EndTurn releases the turn lock.
func (h *Hub) EndTurn(laneID string) {
	r := h.room(laneID)
	r.turnMu.Lock()
	r.busy = false
	r.turnMu.Unlock()
}

// IsTurnBusy reports whether a turn is in progress.
func (h *Hub) IsTurnBusy(laneID string) bool {
	r := h.room(laneID)
	r.turnMu.Lock()
	defer r.turnMu.Unlock()
	return r.busy
}
