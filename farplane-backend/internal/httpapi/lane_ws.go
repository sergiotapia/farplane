package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/farplane/farplane/farplane-backend/internal/lanehub"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

var laneWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (a *api) handleLaneWebSocket(c *gin.Context) { //nolint:gocyclo // multi-branch orchestration; keep under threshold when rewriting
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}

	if _, err := a.store.RequireActiveLaneParticipant(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}

	if a.hub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{jsonKeyError: "stream hub unavailable"})
		return
	}

	conn, err := laneWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &lanehub.Client{
		UserID: principal.User.ID,
		Send:   make(chan []byte, 16),
	}

	a.hub.Subscribe(lane.ID, client)
	defer a.hub.Unsubscribe(lane.ID, client)

	go func() {
		for data := range client.Send {
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}

		_ = conn.Close()
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.Type == "interrupt" {
			a.interruptLaneTurn(lane, principal.User.ID)
		}
	}
}

func (a *api) interruptLaneTurn(lane models.Lane, userID string) {
	if a.runtime == nil || lane.RuntimeID == nil || *lane.RuntimeID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = a.runtime.InterruptTurn(ctx, *lane.RuntimeID)
	if a.hub != nil {
		a.hub.EndTurn(lane.ID)

		role := models.LaneMessageRoleSystem
		body := "Turn interrupted"
		payload, _ := json.Marshal(map[string]any{jsonKeyUserID: userID})

		msg, err := a.store.InsertLaneMessage(ctx, store.InsertLaneMessageInput{
			LaneID:    lane.ID,
			EventType: models.LaneEventStatus,
			Role:      &role,
			Body:      &body,
			Payload:   payload,
		})
		if err == nil {
			a.hub.BroadcastMessage(lane.ID, msg)
		}

		a.hub.BroadcastJSON(lane.ID, map[string]any{
			jsonKeyType: jsonKeyPartial,
			jsonKeyEvent: map[string]any{
				jsonKeyType:   models.LaneEventStatus,
				jsonKeyStatus: "idle",
				"interrupted": true,
			},
		})
	}
}
