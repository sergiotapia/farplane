package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/lanehub"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/runtime"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

const (
	sessionCookieName = "farplane_session"
	contextUserIDKey  = "user_id"
)

type api struct {
	cfg     config.Config
	store   *store.Store
	github  GitHubApp
	runtime runtime.Runtime
	hub     *lanehub.Hub
	catalog *agents.ModelCatalog

	githubMu     sync.RWMutex
	githubForced bool // true when WithGitHubApp injected a test client

	// Optional test hook for manifest code exchange.
	manifestConvert func(ctx context.Context, code string) (githubapp.ManifestApp, error)
}

func newAPI(cfg config.Config, st *store.Store, rt runtime.Runtime, hub *lanehub.Hub) *api {
	a := &api{
		cfg:     cfg,
		store:   st,
		runtime: rt,
		hub:     hub,
		catalog: agents.DefaultModelCatalog(),
	}
	a.github = a.loadGitHubAppLocked()
	return a
}

func (a *api) agentCatalog() *agents.ModelCatalog {
	if a.catalog != nil {
		return a.catalog
	}
	return agents.DefaultModelCatalog()
}

func (a *api) githubApp() GitHubApp {
	a.githubMu.RLock()
	defer a.githubMu.RUnlock()
	return a.github
}

func (a *api) setGitHubApp(client GitHubApp) {
	a.githubMu.Lock()
	defer a.githubMu.Unlock()
	if a.githubForced {
		return
	}
	a.github = client
}

func (a *api) loadGitHubAppLocked() GitHubApp {
	if a.cfg.GitHubAppConfigured() {
		client, err := githubapp.New(githubapp.Config{
			AppID:         a.cfg.GitHubAppID,
			Slug:          a.cfg.GitHubAppSlug,
			PrivateKeyPEM: a.cfg.GitHubAppPrivateKeyPEM,
			WebhookSecret: a.cfg.GitHubAppWebhookSecret,
		})
		if err == nil {
			return client
		}
	}
	if a.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	creds, err := a.store.GetGitHubAppCredentials(ctx, a.cfg.SessionSecret)
	if err != nil {
		return nil
	}
	client, err := githubapp.New(githubapp.Config{
		AppID:         creds.GitHubAppID,
		Slug:          creds.GitHubAppSlug,
		PrivateKeyPEM: creds.PrivateKeyPEM,
		WebhookSecret: creds.WebhookSecret,
	})
	if err != nil {
		return nil
	}
	return client
}

type userJSON struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type organizationJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type meResponse struct {
	User         userJSON         `json:"user"`
	Organization organizationJSON `json:"organization"`
}

func userToJSON(u models.User) userJSON {
	return userJSON{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
	}
}

func meFromStore(u store.UserWithOrg) meResponse {
	return meResponse{
		User: userToJSON(u.User),
		Organization: organizationJSON{
			ID:   u.Organization.ID,
			Name: u.Organization.Name,
			Role: u.Role,
		},
	}
}

func meFromSetup(result store.SetupPasswordResult) meResponse {
	return meResponse{
		User: userToJSON(result.User),
		Organization: organizationJSON{
			ID:   result.Organization.ID,
			Name: result.Organization.Name,
			Role: result.Member.Role,
		},
	}
}

func (a *api) setSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, token, maxAge, "/", "", a.cfg.SessionCookieSecure, true)
}

func (a *api) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", a.cfg.SessionCookieSecure, true)
}

func (a *api) createSessionForUser(c *gin.Context, userID string) (string, time.Time, error) {
	token, err := auth.NewSessionToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().UTC().Add(a.cfg.SessionTTL)
	if _, err := a.store.CreateSession(c.Request.Context(), token, userID, expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// sessionOptional loads the session user id into context when a valid cookie is present.
func (a *api) sessionOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.store == nil {
			c.Next()
			return
		}
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			c.Next()
			return
		}
		userID, err := a.store.GetValidSessionUserID(c.Request.Context(), token, time.Now().UTC())
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "session lookup failed"})
				return
			}
			c.Next()
			return
		}
		c.Set(contextUserIDKey, userID)
		c.Next()
	}
}

// requireSession rejects requests without a valid session.
func (a *api) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get(contextUserIDKey); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Next()
	}
}

func currentUserID(c *gin.Context) (string, bool) {
	v, ok := c.Get(contextUserIDKey)
	if !ok {
		return "", false
	}
	id, ok := v.(string)
	return id, ok && id != ""
}

func writeStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrAlreadySetup):
		c.JSON(http.StatusConflict, gin.H{"error": "setup already completed"})
	case errors.Is(err, store.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "conflict"})
	case errors.Is(err, store.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	}
}

func trimNonEmpty(s string) string {
	return strings.TrimSpace(s)
}
