package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type createLaneInviteRequest struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func (a *api) handleListLaneParticipants(c *gin.Context) {
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
	parts, err := a.store.ListLaneParticipants(c.Request.Context(), lane.ID, true)
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

func (a *api) handleCreateLaneInvite(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if _, err := a.store.RequireLaneOwner(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	var req createLaneInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	userID := strings.TrimSpace(req.UserID)
	email := strings.TrimSpace(req.Email)
	if userID == "" && email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id or email is required"})
		return
	}
	var invitedUserID *string
	var invitedEmail *string
	if userID != "" {
		// Must be an org member.
		members, err := a.store.ListOrganizationMembersForInvite(c.Request.Context(), principal.Organization.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
			return
		}
		found := false
		for _, m := range members {
			if m.ID == userID {
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user is not an organization member"})
			return
		}
		invitedUserID = &userID
	}
	if email != "" {
		invitedEmail = &email
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	inv, err := a.store.CreateLaneInvite(c.Request.Context(), store.CreateLaneInviteInput{
		LaneID:          lane.ID,
		Email:           invitedEmail,
		InvitedUserID:   invitedUserID,
		InvitedByUserID: principal.User.ID,
		ExpiresAt:       &expires,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invite"})
		return
	}
	c.JSON(http.StatusCreated, laneInviteJSON(inv, a.cfg.AppBaseURL))
}

func (a *api) handleListLaneInvites(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if _, err := a.store.RequireLaneOwner(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	invites, err := a.store.ListLaneInvites(c.Request.Context(), lane.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list invites"})
		return
	}
	out := make([]gin.H, 0, len(invites))
	for _, inv := range invites {
		out = append(out, laneInviteJSON(inv, a.cfg.AppBaseURL))
	}
	c.JSON(http.StatusOK, gin.H{"invites": out})
}

func (a *api) handleKickLaneParticipant(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if _, err := a.store.RequireLaneOwner(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	target := c.Param("user_id")
	if target == principal.User.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot kick yourself"})
		return
	}
	if err := a.store.KickLaneParticipant(c.Request.Context(), lane.ID, target, principal.User.ID); err != nil {
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
		"id":                  inv.ID,
		"lane_id":             inv.LaneID,
		"token":               inv.Token,
		"email":               inv.Email,
		"invited_user_id":     inv.InvitedUserID,
		"invited_by_user_id":  inv.InvitedByUserID,
		"expires_at":          inv.ExpiresAt,
		"accepted_at":         inv.AcceptedAt,
		"accepted_by_user_id": inv.AcceptedByUserID,
		"revoked_at":          inv.RevokedAt,
		"created_at":          inv.CreatedAt,
		"accept_url":          acceptURL,
	}
}
