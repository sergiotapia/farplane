package httpapi

import (
	"errors"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

func (a *api) handleMe(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return
	}
	principal, err := a.store.GetUserWithOrgByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return
	}
	c.JSON(http.StatusOK, meFromStore(principal))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *api) handleLogin(c *gin.Context) {
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	email, emailOK := normalizeEmail(req.Email)
	if !emailOK || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
		return
	}
	if len(req.Password) > auth.MaxPasswordBytes {
		_ = auth.CheckPassword(dummyPasswordHash(), req.Password)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	user, err := a.store.GetUserByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = auth.CheckPassword(dummyPasswordHash(), req.Password)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}
	hash := dummyPasswordHash()
	if user.PasswordHash != nil {
		hash = *user.PasswordHash
	}
	if !auth.CheckPassword(hash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	principal, err := a.store.GetUserWithOrgByUserID(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return
	}

	token, expiresAt, err := a.createSessionForUser(c, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}
	a.setSessionCookie(c, token, expiresAt)
	c.JSON(http.StatusOK, meFromStore(principal))
}

func (a *api) handleLogout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookieName); err == nil && token != "" && a.store != nil {
		_ = a.store.DeleteSessionByToken(c.Request.Context(), token)
	}
	a.clearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

func dummyPasswordHash() string {
	dummyHashOnce.Do(func() {
		hash, err := auth.HashPassword("farplane-dummy-password-not-a-real-secret")
		if err != nil {
			panic(err)
		}
		dummyHash = hash
	})
	return dummyHash
}
