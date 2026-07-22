package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/lanehub"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/runtime"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type postLaneMessageRequest struct {
	Text string `json:"text"`
}

func (a *api) handleListLaneMessages(c *gin.Context) {
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
	msgs, err := a.store.ListLaneMessages(c.Request.Context(), lane.ID, 0, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}
	out := make([]gin.H, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, lanehub.MessageDTO(m))
	}
	c.JSON(http.StatusOK, gin.H{"messages": out})
}

func (a *api) handlePostLaneMessage(c *gin.Context) {
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
	var req postLaneMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}
	if a.hub != nil && !a.hub.TryBeginTurn(lane.ID) {
		c.JSON(http.StatusConflict, gin.H{"error": "a turn is already running"})
		return
	}
	role := models.LaneMessageRoleUser
	uid := principal.User.ID
	msg, err := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
		LaneID:       lane.ID,
		EventType:    models.LaneEventUserMessage,
		Role:         &role,
		AuthorUserID: &uid,
		Body:         &text,
	})
	if err != nil {
		if a.hub != nil {
			a.hub.EndTurn(lane.ID)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save message"})
		return
	}
	if a.hub != nil {
		a.hub.BroadcastMessage(lane.ID, msg)
	}

	go a.runAgentTurn(context.Background(), lane, msg, principal.User.DisplayName)

	c.JSON(http.StatusAccepted, lanehub.MessageDTO(msg))
}

func (a *api) runAgentTurn(parent context.Context, lane models.Lane, userMsg models.LaneMessage, authorName string) {
	defer func() {
		if a.hub != nil {
			a.hub.EndTurn(lane.ID)
		}
	}()
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()
	if a.runtime == nil || lane.RuntimeID == nil || *lane.RuntimeID == "" {
		role := models.LaneMessageRoleAssistant
		body := "Lane runtime is not running; message saved but not sent to an agent."
		msg, err := a.store.InsertLaneMessage(ctx, store.InsertLaneMessageInput{
			LaneID:    lane.ID,
			EventType: models.LaneEventAssistantMessage,
			Role:      &role,
			Body:      &body,
		})
		if err == nil && a.hub != nil {
			a.hub.BroadcastMessage(lane.ID, msg)
		}
		return
	}

	_ = a.runtime.Start(ctx, *lane.RuntimeID)
	_ = a.runtime.EnsureAgentBridge(ctx, *lane.RuntimeID)

	stream, err := a.runtime.OpenAgentStream(ctx, *lane.RuntimeID)
	sessionID := ""
	if lane.AgentProviderSessionID != nil {
		sessionID = *lane.AgentProviderSessionID
	}
	body := ""
	if userMsg.Body != nil {
		body = *userMsg.Body
	}
	authorID := ""
	if userMsg.AuthorUserID != nil {
		authorID = *userMsg.AuthorUserID
	}
	_ = a.runtime.SendUserTurn(ctx, *lane.RuntimeID, runtime.UserTurn{
		Type:              "user_turn",
		LaneID:            lane.ID,
		MessageID:         userMsg.ID,
		AuthorUserID:      authorID,
		AuthorDisplayName: authorName,
		Text:              body,
		Provider:          lane.AgentProvider,
		ProviderSessionID: sessionID,
	})
	if err == nil {
		a.consumeAgentStream(ctx, lane.ID, stream)
	}
}

func (a *api) consumeAgentStream(ctx context.Context, laneID string, stream runtime.AgentStream) {
	defer stream.Close()
	var assistantBuf strings.Builder
	seenRunning := false
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-stream.Events():
			if !ok {
				a.flushAssistantBuffer(ctx, laneID, &assistantBuf)
				return
			}
			switch ev.Type {
			case models.LaneEventProviderSession:
				if ev.ProviderSessionID != "" {
					_ = a.store.SetLaneAgentProviderSessionID(ctx, laneID, ev.ProviderSessionID)
				}
			case models.LaneEventAssistantMessage:
				if ev.Body != "" {
					assistantBuf.WriteString(ev.Body)
				}
				if a.hub != nil {
					a.hub.BroadcastJSON(laneID, map[string]any{
						"type":  "partial",
						"event": ev,
					})
				}
				if ev.Done {
					a.flushAssistantBuffer(ctx, laneID, &assistantBuf)
				}
			case models.LaneEventStatus:
				if a.hub != nil {
					a.hub.BroadcastJSON(laneID, map[string]any{
						"type":  "partial",
						"event": ev,
					})
				}
				if ev.Status == "running" {
					seenRunning = true
				}
				if ev.Status == "idle" && seenRunning {
					a.flushAssistantBuffer(ctx, laneID, &assistantBuf)
					return
				}
			case models.LaneEventToolProgress, models.LaneEventToolCall:
				if a.hub != nil {
					a.hub.BroadcastJSON(laneID, map[string]any{
						"type":  "partial",
						"event": ev,
					})
				}
				payload, _ := json.Marshal(ev)
				body := ev.Body
				_, _ = a.store.InsertLaneMessage(ctx, store.InsertLaneMessageInput{
					LaneID:    laneID,
					EventType: ev.Type,
					Body:      &body,
					Payload:   payload,
				})
			}
		}
	}
}

func (a *api) flushAssistantBuffer(ctx context.Context, laneID string, buf *strings.Builder) {
	if buf.Len() == 0 {
		return
	}
	role := models.LaneMessageRoleAssistant
	body := buf.String()
	msg, err := a.store.InsertLaneMessage(ctx, store.InsertLaneMessageInput{
		LaneID:    laneID,
		EventType: models.LaneEventAssistantMessage,
		Role:      &role,
		Body:      &body,
	})
	if err == nil && a.hub != nil {
		a.hub.BroadcastMessage(laneID, msg)
	}
	buf.Reset()
}
