package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type laneInviteSignupRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func (a *api) handleGetLaneInvite(c *gin.Context) {
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{jsonKeyError: errDatabaseUnavailable})
		return
	}

	preview, err := a.store.GetLaneInvitePreview(c.Request.Context(), c.Param("token"))
	if err != nil {
		writeStoreError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":                   preview.Token,
		jsonKeyLaneID:             preview.LaneID,
		"lane_name":               preview.LaneName,
		"invited_by_display_name": preview.InvitedByDisplayName,
		"expires_at":              preview.ExpiresAt,
		"pending":                 preview.Pending,
		"accept_url":              strings.TrimRight(a.cfg.AppBaseURL, "/") + "/lane-invites/" + preview.Token,
	})
}

func (a *api) handleSignupLaneInvite(c *gin.Context) { //nolint:gocyclo,funlen // multi-branch orchestration; keep under threshold when rewriting
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{jsonKeyError: errDatabaseUnavailable})
		return
	}

	var req laneInviteSignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: errInvalidRequestBody})
		return
	}

	email, emailOK := normalizeEmail(req.Email)
	displayName := trimNonEmpty(req.DisplayName)
	password := req.Password

	if !emailOK {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "email is invalid"})
		return
	}

	if displayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "display_name is required"})
		return
	}

	if utf8.RuneCountInString(displayName) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "display_name is too long"})
		return
	}

	if len(password) < auth.MinPasswordLength {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "password must be at least 8 bytes"})
		return
	}

	if len(password) > auth.MaxPasswordBytes {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: errPasswordTooLong})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordTooLong) {
			c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: errPasswordTooLong})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to hash password"})

		return
	}

	token, err := auth.NewSessionToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to create session"})
		return
	}

	expires := time.Now().UTC().Add(a.cfg.SessionTTL)

	result, err := a.store.SignUpAndAcceptLaneInvite(c.Request.Context(), store.LaneInviteSignupInput{
		Token:            c.Param("token"),
		Email:            email,
		DisplayName:      displayName,
		PasswordHash:     hash,
		SessionToken:     token,
		SessionExpiresAt: expires,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{
				jsonKeyError: "invite is not available or an account with this email already exists; sign in instead",
			})

			return
		}

		writeStoreError(c, err)

		return
	}

	a.setSessionCookie(c, token, expires)
	a.broadcastInviteJoined(result.Invite.LaneID, result.User)
	c.JSON(http.StatusCreated, gin.H{
		jsonKeyLaneID: result.Invite.LaneID,
		"invite":      laneInviteJSON(result.Invite, a.cfg.AppBaseURL),
		"user": gin.H{
			"id":               result.User.ID,
			jsonKeyEmail:       result.User.Email,
			jsonKeyDisplayName: result.User.DisplayName,
		},
	})
}

func (a *api) broadcastInviteJoined(laneID string, user models.User) {
	if a.hub == nil || a.store == nil {
		return
	}

	role := models.LaneMessageRoleSystem
	body := user.DisplayName + " joined the Lane"
	uid := user.ID

	msg, err := a.store.InsertLaneMessage(context.Background(), store.InsertLaneMessageInput{
		LaneID:       laneID,
		EventType:    models.LaneEventParticipantJoined,
		Role:         &role,
		AuthorUserID: &uid,
		Body:         &body,
	})
	if err == nil {
		a.hub.BroadcastMessage(laneID, msg)
	}
}
