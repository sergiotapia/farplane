package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

const githubManifestStateTTL = 30 * time.Minute

type manifestStartRequest struct {
	GitHubOrganizationLogin string `json:"github_organization_login"`
}

func (a *api) handleGitHubManifestStart(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	if !isOrgAdmin(principal.Role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only owners and admins can create the GitHub App"})
		return
	}
	if a.githubApp() != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "github app is already configured"})
		return
	}
	if !config.IsPublicAPIBaseURL(a.apiBaseURL()) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "APP_API_BASE_URL must be a public https URL (for example your ngrok https URL). GitHub rejects localhost, and http is not allowed.",
			"api_base_url": a.apiBaseURL(),
		})
		return
	}

	var req manifestStartRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
	}
	orgLogin := strings.TrimSpace(req.GitHubOrganizationLogin)
	if utf8.RuneCountInString(orgLogin) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "github_organization_login is too long"})
		return
	}

	nonce, err := auth.NewOAuthNonce()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start manifest"})
		return
	}
	state, err := auth.SignGitHubInstallState(a.cfg.SessionSecret, auth.GitHubInstallState{
		OrganizationID: principal.Organization.ID,
		UserID:         principal.User.ID,
		Nonce:          nonce,
		ExpiresAtUnix:  time.Now().UTC().Add(githubManifestStateTTL).Unix(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start manifest"})
		return
	}

	manifest := githubapp.BuildManifest(
		a.cfg.AppBaseURL,
		a.apiBaseURL(),
		principal.Organization.Name,
	)
	raw, err := json.Marshal(manifest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build manifest"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"action":   githubapp.ManifestRegisterURL(orgLogin),
		"manifest": string(raw),
		"state":    state,
	})
}

func (a *api) handleGitHubManifestCallback(c *gin.Context) {
	if a.store == nil {
		a.redirectGitHubError(c, "database_unavailable")
		return
	}
	if errCode := trimNonEmpty(c.Query("error")); errCode != "" {
		a.redirectGitHubError(c, "github_denied")
		return
	}

	stateRaw := trimNonEmpty(c.Query("state"))
	state, err := auth.ParseGitHubInstallState(a.cfg.SessionSecret, stateRaw, time.Now().UTC())
	if err != nil {
		a.redirectGitHubError(c, "invalid_state")
		return
	}
	code := trimNonEmpty(c.Query("code"))
	if code == "" {
		a.redirectGitHubError(c, "missing_manifest_code")
		return
	}

	principal, err := a.store.GetUserWithOrgByUserID(c.Request.Context(), state.UserID)
	if err != nil {
		a.redirectGitHubError(c, "manifest_save_failed")
		return
	}
	if principal.Organization.ID != state.OrganizationID || !isOrgAdmin(principal.Role) {
		a.redirectGitHubError(c, "manifest_forbidden")
		return
	}

	app, err := a.convertManifest(c.Request.Context(), code)
	if err != nil {
		a.redirectGitHubError(c, "manifest_exchange_failed")
		return
	}

	_, err = a.store.InsertGitHubAppCredentials(c.Request.Context(), store.InsertGitHubAppCredentialsInput{
		GitHubAppID:     app.ID,
		GitHubAppSlug:   app.Slug,
		PrivateKeyPEM:   app.PEM,
		WebhookSecret:   app.WebhookSecret,
		ClientID:        app.ClientID,
		ClientSecret:    app.ClientSecret,
		CreatedByUserID: state.UserID,
		EncryptionKey:   a.cfg.SessionSecret,
	})
	if err != nil {
		if errors.Is(err, store.ErrGitHubAppCredentialsExist) {
			a.redirectGitHubError(c, "github_app_already_configured")
			return
		}
		a.redirectGitHubError(c, "manifest_save_failed")
		return
	}

	client, err := githubapp.New(githubapp.Config{
		AppID:         app.ID,
		Slug:          app.Slug,
		PrivateKeyPEM: app.PEM,
		WebhookSecret: app.WebhookSecret,
	})
	if err != nil {
		a.redirectGitHubError(c, "manifest_client_failed")
		return
	}
	a.setGitHubApp(client)

	c.Redirect(http.StatusFound, a.cfg.AppBaseURL+"/settings/github?github=app_created")
}

func (a *api) convertManifest(ctx context.Context, code string) (githubapp.ManifestApp, error) {
	if a.manifestConvert != nil {
		return a.manifestConvert(ctx, code)
	}
	return githubapp.ConvertManifest(ctx, nil, "", code)
}

func (a *api) apiBaseURL() string {
	if strings.TrimSpace(a.cfg.AppAPIBaseURL) != "" {
		return strings.TrimRight(a.cfg.AppAPIBaseURL, "/")
	}
	return "http://localhost:8080"
}

func isOrgAdmin(role string) bool {
	return role == models.OrganizationRoleOwner || role == models.OrganizationRoleAdmin
}
