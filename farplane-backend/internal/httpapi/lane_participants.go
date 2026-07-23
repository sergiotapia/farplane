package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type addLaneParticipantRequest struct {
	UserID string `json:"user_id"`
}

func (a *api) requireLaneForParticipant(c *gin.Context) (models.Lane, bool) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return models.Lane{}, false
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return models.Lane{}, false
	}
	if lane.OrganizationID != principal.Organization.ID || lane.Status == models.LaneStatusDestroyed {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return models.Lane{}, false
	}
	if _, err := a.store.RequireActiveLaneParticipant(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return models.Lane{}, false
	}
	return lane, true
}

func (a *api) handleListLaneParticipants(c *gin.Context) {
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	parts, err := a.store.ListLaneParticipants(c.Request.Context(), lane.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list participants"})
		return
	}
	out := make([]gin.H, 0, len(parts))
	for _, p := range parts {
		user, _ := a.store.GetUserByID(c.Request.Context(), p.UserID)
		out = append(out, gin.H{
			"id":           p.ID,
			"lane_id":      p.LaneID,
			"user_id":      p.UserID,
			"role":         p.Role,
			"joined_at":    p.JoinedAt,
			"display_name": user.DisplayName,
			"email":        user.Email,
		})
	}
	c.JSON(http.StatusOK, gin.H{"participants": out})
}

func (a *api) handleAddLaneParticipant(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	var req addLaneParticipantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}
	exists, err := a.store.OrganizationMemberExists(c.Request.Context(), principal.Organization.ID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check membership"})
		return
	}
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is not an organization member"})
		return
	}
	p, err := a.store.AddLaneParticipant(c.Request.Context(), lane.ID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add participant"})
		return
	}
	if a.hub != nil {
		user, _ := a.store.GetUserByID(c.Request.Context(), userID)
		role := models.LaneMessageRoleSystem
		body := user.DisplayName + " joined the Lane"
		uid := userID
		msg, _ := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
			LaneID:       lane.ID,
			EventType:    models.LaneEventParticipantJoined,
			Role:         &role,
			AuthorUserID: &uid,
			Body:         &body,
		})
		a.hub.BroadcastMessage(lane.ID, msg)
	}
	user, _ := a.store.GetUserByID(c.Request.Context(), p.UserID)
	c.JSON(http.StatusCreated, gin.H{
		"id":           p.ID,
		"lane_id":      p.LaneID,
		"user_id":      p.UserID,
		"role":         p.Role,
		"joined_at":    p.JoinedAt,
		"display_name": user.DisplayName,
		"email":        user.Email,
	})
}

func (a *api) handleRemoveLaneParticipant(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	target := c.Param("user_id")
	if target == principal.User.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove yourself; use leave"})
		return
	}
	if err := a.store.RemoveLaneParticipant(c.Request.Context(), lane.ID, target); err != nil {
		writeStoreError(c, err)
		return
	}
	if a.hub != nil {
		a.hub.DropUser(lane.ID, target)
		role := models.LaneMessageRoleSystem
		body := "Participant removed"
		payload, _ := json.Marshal(map[string]any{"user_id": target, "by": principal.User.ID})
		msg, _ := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
			LaneID:    lane.ID,
			EventType: models.LaneEventParticipantRemoved,
			Role:      &role,
			Body:      &body,
			Payload:   payload,
		})
		a.hub.BroadcastMessage(lane.ID, msg)
	}
	c.Status(http.StatusNoContent)
}

func (a *api) handleLeaveLane(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	if err := a.store.LeaveLane(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	if a.hub != nil {
		a.hub.DropUser(lane.ID, principal.User.ID)
		role := models.LaneMessageRoleSystem
		body := principal.User.DisplayName + " left the Lane"
		uid := principal.User.ID
		payload, _ := json.Marshal(map[string]any{"user_id": uid})
		msg, _ := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
			LaneID:       lane.ID,
			EventType:    models.LaneEventParticipantRemoved,
			Role:         &role,
			AuthorUserID: &uid,
			Body:         &body,
			Payload:      payload,
		})
		a.hub.BroadcastMessage(lane.ID, msg)
	}
	c.Status(http.StatusNoContent)
}

func (a *api) handleGetActiveLaneInvite(c *gin.Context) {
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	inv, err := a.store.GetActiveLaneInvite(c.Request.Context(), lane.ID)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, laneInviteJSON(inv, a.cfg.AppBaseURL))
}

func (a *api) handleCreateLaneInvite(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	inv, err := a.store.EnsureLaneInvite(c.Request.Context(), store.CreateLaneInviteInput{
		LaneID:          lane.ID,
		InvitedByUserID: principal.User.ID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite"})
		return
	}
	c.JSON(http.StatusOK, laneInviteJSON(inv, a.cfg.AppBaseURL))
}

func (a *api) handleRegenerateLaneInvite(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	inv, err := a.store.RegenerateLaneInvite(c.Request.Context(), lane.ID, principal.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to regenerate invite"})
		return
	}
	c.JSON(http.StatusOK, laneInviteJSON(inv, a.cfg.AppBaseURL))
}

func (a *api) handleRevokeActiveLaneInvite(c *gin.Context) {
	lane, ok := a.requireLaneForParticipant(c)
	if !ok {
		return
	}
	if err := a.store.RevokeActiveLaneInvite(c.Request.Context(), lane.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) handleAcceptLaneInvite(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	inv, err := a.store.AcceptLaneInvite(c.Request.Context(), c.Param("token"), principal.User.ID)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if a.hub != nil {
		role := models.LaneMessageRoleSystem
		body := principal.User.DisplayName + " joined the Lane"
		uid := principal.User.ID
		msg, _ := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
			LaneID:       inv.LaneID,
			EventType:    models.LaneEventParticipantJoined,
			Role:         &role,
			AuthorUserID: &uid,
			Body:         &body,
		})
		a.hub.BroadcastMessage(inv.LaneID, msg)
	}
	c.JSON(http.StatusOK, laneInviteJSON(inv, a.cfg.AppBaseURL))
}

func (a *api) handleListOrganizationMembers(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	members, err := a.store.ListOrganizationMembersForInvite(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}
	out := make([]gin.H, 0, len(members))
	for _, u := range members {
		out = append(out, gin.H{
			"id":           u.ID,
			"email":        u.Email,
			"display_name": u.DisplayName,
			"avatar_url":   u.AvatarURL,
		})
	}
	c.JSON(http.StatusOK, gin.H{"members": out})
}

func laneInviteJSON(inv models.LaneInvite, appBaseURL string) gin.H {
	acceptURL := strings.TrimRight(appBaseURL, "/") + "/lane-invites/" + inv.Token
	return gin.H{
		"id":                 inv.ID,
		"lane_id":            inv.LaneID,
		"token":              inv.Token,
		"invited_by_user_id": inv.InvitedByUserID,
		"expires_at":         inv.ExpiresAt,
		"revoked_at":         inv.RevokedAt,
		"created_at":         inv.CreatedAt,
		"accept_url":         acceptURL,
	}
}
