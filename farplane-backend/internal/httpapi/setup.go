package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type setupStatusResponse struct {
	NeedsSetup            bool `json:"needs_setup"`
	GoogleOAuthConfigured bool `json:"google_oauth_configured"`
	SetupTokenRequired    bool `json:"setup_token_required"`
}

func (a *api) handleSetupStatus(c *gin.Context) {
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{jsonKeyError: errDatabaseUnavailable})
		return
	}

	needs, err := a.store.NeedsSetup(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: errSetupStatus})
		return
	}

	c.JSON(http.StatusOK, setupStatusResponse{
		NeedsSetup:            needs,
		GoogleOAuthConfigured: a.cfg.GoogleOAuthConfigured(),
		SetupTokenRequired:    a.cfg.SetupToken != "",
	})
}

type setupRequest struct {
	OrganizationName string `json:"organization_name"`
	Email            string `json:"email"`
	DisplayName      string `json:"display_name"`
	Password         string `json:"password"`
	SetupToken       string `json:"setup_token"`
}

func (a *api) handleSetup(c *gin.Context) { //nolint:gocyclo,funlen // multi-branch orchestration; keep under threshold when rewriting
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{jsonKeyError: errDatabaseUnavailable})
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: errInvalidRequestBody})
		return
	}

	if !a.setupTokenOK(c, req.SetupToken) {
		c.JSON(http.StatusUnauthorized, gin.H{jsonKeyError: "invalid setup token"})
		return
	}

	orgName := trimNonEmpty(req.OrganizationName)
	email, emailOK := normalizeEmail(req.Email)
	displayName := trimNonEmpty(req.DisplayName)
	password := req.Password

	if orgName == "" {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "organization_name is required"})
		return
	}

	if utf8.RuneCountInString(orgName) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "organization_name is too long"})
		return
	}

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

	needs, err := a.store.NeedsSetup(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: errSetupStatus})
		return
	}

	if !needs {
		c.JSON(http.StatusConflict, gin.H{jsonKeyError: "setup already completed"})
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

	sessionExpires := time.Now().UTC().Add(a.cfg.SessionTTL)

	result, err := a.store.CompletePasswordSetup(c.Request.Context(), store.SetupPasswordInput{
		OrganizationName: orgName,
		Email:            email,
		DisplayName:      displayName,
		PasswordHash:     hash,
		SessionToken:     token,
		SessionExpiresAt: sessionExpires,
	})
	if err != nil {
		writeStoreError(c, err)
		return
	}

	a.setSessionCookie(c, token, sessionExpires)
	c.JSON(http.StatusCreated, meFromSetup(result))
}

func (a *api) setupTokenOK(c *gin.Context, bodyToken string) bool {
	expected := a.cfg.SetupToken
	if expected == "" {
		return true
	}

	provided := trimNonEmpty(bodyToken)
	if provided == "" {
		provided = trimNonEmpty(c.GetHeader("X-Farplane-Setup-Token"))
	}

	if provided == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
