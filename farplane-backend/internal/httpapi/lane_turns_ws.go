package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/farplane/farplane/farplane-backend/internal/lanehub"
)

// handleLanesTurnWebSocket streams turn_running updates for the caller's lanes.
// Register before /lanes/:id/ws so "turns" is not captured as an id.
func (a *api) handleLanesTurnWebSocket(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	if a.hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "stream hub unavailable"})
		return
	}
	conn, err := laneWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &lanehub.StatusClient{
		UserID: principal.User.ID,
		Send:   make(chan []byte, 32),
	}
	a.hub.SubscribeStatus(client)
	defer a.hub.UnsubscribeStatus(client)

	go func() {
		for data := range client.Send {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				_ = conn.Close()
				return
			}
		}
		_ = conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg struct {
			Type    string   `json:"type"`
			LaneIDs []string `json:"lane_ids"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type != "watch" {
			continue
		}
		allowed := a.filterParticipantLaneIDs(
			c.Request.Context(),
			principal.Organization.ID,
			principal.User.ID,
			msg.LaneIDs,
		)
		a.hub.SetStatusWatches(client, allowed)
		snapshot := a.hub.TurnSnapshot(allowed)
		payload, err := json.Marshal(map[string]any{
			"type":  "snapshot",
			"turns": snapshot,
		})
		if err != nil {
			continue
		}
		select {
		case client.Send <- payload:
		default:
		}
	}
}

func (a *api) filterParticipantLaneIDs(
	ctx context.Context,
	organizationID, userID string,
	requested []string,
) []string {
	if len(requested) == 0 {
		return nil
	}
	grouped, err := a.store.ListLanesGrouped(ctx, organizationID, userID)
	if err != nil {
		return nil
	}
	allowed := make(map[string]struct{})
	for _, group := range grouped.Projects {
		for _, lane := range group.Lanes {
			allowed[lane.ID] = struct{}{}
		}
	}
	for _, lane := range grouped.ScratchLanes {
		allowed[lane.ID] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		if id == "" {
			continue
		}
		if _, ok := allowed[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
