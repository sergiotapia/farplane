package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

const githubInstallStateTTL = 15 * time.Minute

// GitHubApp is the GitHub App surface used by HTTP handlers (mockable in tests).
type GitHubApp interface {
	InstallURL(state string) string
	GetInstallation(ctx context.Context, installationID int64) (githubapp.Installation, error)
	CreateInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error)
	ListInstallationRepositories(ctx context.Context, installationToken string) ([]githubapp.Repository, error)
	VerifyWebhookSignature(body []byte, signatureHeader string) bool
}

func (a *api) handleGitHubInstallStart(c *gin.Context) {
	gh := a.githubApp()
	if gh == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "github app is not configured"})
		return
	}
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	nonce, err := auth.NewOAuthNonce()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start github install"})
		return
	}
	state, err := auth.SignGitHubInstallState(a.cfg.SessionSecret, auth.GitHubInstallState{
		OrganizationID: principal.Organization.ID,
		UserID:         principal.User.ID,
		Nonce:          nonce,
		ExpiresAtUnix:  time.Now().UTC().Add(githubInstallStateTTL).Unix(),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start github install"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": gh.InstallURL(state)})
}

func (a *api) handleGitHubCallback(c *gin.Context) {
	gh := a.githubApp()
	if gh == nil || a.store == nil {
		a.redirectGitHubError(c, "github_app_not_configured")
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

	principal, err := a.store.GetUserWithOrgByUserID(c.Request.Context(), state.UserID)
	if err != nil {
		a.redirectGitHubError(c, "install_save_failed")
		return
	}
	if principal.Organization.ID != state.OrganizationID {
		a.redirectGitHubError(c, "install_forbidden")
		return
	}

	installationID, err := strconv.ParseInt(trimNonEmpty(c.Query("installation_id")), 10, 64)
	if err != nil || installationID <= 0 {
		a.redirectGitHubError(c, "missing_installation")
		return
	}

	ghInst, err := gh.GetInstallation(c.Request.Context(), installationID)
	if err != nil {
		a.redirectGitHubError(c, "installation_lookup_failed")
		return
	}

	selection := ghInst.RepositorySelection
	if selection != models.GitHubRepositorySelectionAll && selection != models.GitHubRepositorySelectionSelected {
		selection = models.GitHubRepositorySelectionSelected
	}
	accountType := ghInst.Account.Type
	if accountType != models.GitHubAccountTypeUser && accountType != models.GitHubAccountTypeOrganization {
		a.redirectGitHubError(c, "installation_lookup_failed")
		return
	}

	var suspendedAt *time.Time
	if ghInst.SuspendedAt != nil && *ghInst.SuspendedAt != "" {
		if t, parseErr := time.Parse(time.RFC3339, *ghInst.SuspendedAt); parseErr == nil {
			utc := t.UTC()
			suspendedAt = &utc
		}
	}

	inst, err := a.store.UpsertGitHubInstallation(c.Request.Context(), store.UpsertGitHubInstallationInput{
		OrganizationID:       state.OrganizationID,
		GitHubInstallationID: ghInst.ID,
		GitHubAccountID:      ghInst.Account.ID,
		GitHubAccountLogin:   ghInst.Account.Login,
		GitHubAccountType:    accountType,
		RepositorySelection:  selection,
		ConnectedByUserID:    state.UserID,
		SuspendedAt:          suspendedAt,
	})
	if err != nil {
		if errors.Is(err, store.ErrGitHubInstallationOwned) {
			a.redirectGitHubError(c, "installation_owned")
			return
		}
		a.redirectGitHubError(c, "install_save_failed")
		return
	}

	if err := a.syncInstallationRepositories(c.Request.Context(), inst); err != nil {
		a.redirectGitHubError(c, "repo_sync_failed")
		return
	}

	c.Redirect(http.StatusFound, a.cfg.AppBaseURL+"/settings/github?github=connected")
}

func (a *api) handleListGitHubInstallations(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	configured := a.githubApp() != nil
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return
	}
	installations, err := a.store.ListGitHubInstallations(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list installations"})
		return
	}
	items := make([]gin.H, 0, len(installations))
	for _, inst := range installations {
		items = append(items, gin.H{
			"id":                     inst.ID,
			"github_installation_id": inst.GitHubInstallationID,
			"github_account_id":      inst.GitHubAccountID,
			"github_account_login":   inst.GitHubAccountLogin,
			"github_account_type":    inst.GitHubAccountType,
			"repository_selection":   inst.RepositorySelection,
			"connected_by_user_id":   inst.ConnectedByUserID,
			"suspended":              inst.SuspendedAt != nil,
			"created_at":             inst.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"configured":          configured,
		"api_base_url":        a.apiBaseURL(),
		"api_base_url_public": config.IsPublicAPIBaseURL(a.apiBaseURL()),
		"installations":       items,
	})
}

func (a *api) handleDisconnectGitHubInstallation(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	inst, err := a.store.GetGitHubInstallationByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if inst.OrganizationID != principal.Organization.ID || inst.UninstalledAt != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	canDisconnect := inst.ConnectedByUserID == principal.User.ID ||
		principal.Role == models.OrganizationRoleOwner ||
		principal.Role == models.OrganizationRoleAdmin
	if !canDisconnect {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := a.store.MarkGitHubInstallationUninstalled(c.Request.Context(), inst.GitHubInstallationID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disconnect"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) handleListGitHubRepositories(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	refresh := c.Query("refresh") == "1" || strings.EqualFold(c.Query("refresh"), "true")
	if refresh && a.githubApp() != nil {
		installations, err := a.store.ListGitHubInstallations(c.Request.Context(), principal.Organization.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list installations"})
			return
		}
		for _, inst := range installations {
			if inst.SuspendedAt != nil {
				continue
			}
			_ = a.syncInstallationRepositories(c.Request.Context(), inst)
		}
	}

	repos, err := a.store.ListPickableGitHubRepositories(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list repositories"})
		return
	}
	items := make([]gin.H, 0, len(repos))
	for _, repo := range repos {
		items = append(items, gin.H{
			"github_repository_id":   repo.GitHubRepositoryID,
			"full_name":              repo.FullName,
			"default_branch":         repo.DefaultBranch,
			"private":                repo.Private,
			"html_url":               repo.HTMLURL,
			"github_installation_id": repo.GitHubInstallationID,
			"github_account_type":    repo.GitHubAccountType,
			"github_account_login":   repo.GitHubAccountLogin,
		})
	}
	c.JSON(http.StatusOK, gin.H{"repositories": items})
}

func (a *api) handleGitHubWebhook(c *gin.Context) {
	gh := a.githubApp()
	if gh == nil || a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "github app is not configured"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !gh.VerifyWebhookSignature(body, c.GetHeader("X-Hub-Signature-256")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	event := c.GetHeader("X-GitHub-Event")
	payload, err := githubapp.ParseWebhookInstallation(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	var opErr error
	switch event {
	case "installation":
		switch payload.Action {
		case "deleted":
			opErr = a.store.MarkGitHubInstallationUninstalled(c.Request.Context(), payload.Installation.ID)
		case "suspend":
			now := time.Now().UTC()
			opErr = a.store.SetGitHubInstallationSuspended(c.Request.Context(), payload.Installation.ID, &now)
		case "unsuspend":
			opErr = a.store.SetGitHubInstallationSuspended(c.Request.Context(), payload.Installation.ID, nil)
			if opErr == nil {
				inst, err := a.store.GetGitHubInstallationByGitHubID(c.Request.Context(), payload.Installation.ID)
				if err == nil && inst.UninstalledAt == nil {
					opErr = a.syncInstallationRepositories(c.Request.Context(), inst)
				}
			}
		case "created", "new_permissions_accepted":
			inst, err := a.store.GetGitHubInstallationByGitHubID(c.Request.Context(), payload.Installation.ID)
			if err == nil && inst.UninstalledAt == nil {
				opErr = a.syncInstallationRepositories(c.Request.Context(), inst)
			} else if err != nil && !errors.Is(err, store.ErrNotFound) {
				opErr = err
			}
		}
	case "installation_repositories":
		inst, err := a.store.GetGitHubInstallationByGitHubID(c.Request.Context(), payload.Installation.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				c.Status(http.StatusNoContent)
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
			return
		}
		if inst.UninstalledAt != nil {
			c.Status(http.StatusNoContent)
			return
		}
		if len(payload.RepositoriesRemoved) > 0 {
			ids := make([]int64, 0, len(payload.RepositoriesRemoved))
			for _, repo := range payload.RepositoriesRemoved {
				ids = append(ids, repo.ID)
			}
			opErr = a.store.SoftRemoveGitHubRepositories(c.Request.Context(), inst.ID, ids)
		}
		if opErr == nil && (len(payload.RepositoriesAdded) > 0 || payload.Action == "added") {
			opErr = a.syncInstallationRepositories(c.Request.Context(), inst)
		}
	}

	if opErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook processing failed"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) syncInstallationRepositories(ctx context.Context, inst models.GitHubInstallation) error {
	gh := a.githubApp()
	if gh == nil {
		return errors.New("github app is not configured")
	}
	token, _, err := gh.CreateInstallationToken(ctx, inst.GitHubInstallationID)
	if err != nil {
		return err
	}
	repos, err := gh.ListInstallationRepositories(ctx, token)
	if err != nil {
		return err
	}
	sync := make([]store.GitHubRepoSync, 0, len(repos))
	for _, repo := range repos {
		sync = append(sync, store.GitHubRepoSync{
			GitHubRepositoryID: repo.ID,
			FullName:           repo.FullName,
			DefaultBranch:      repo.DefaultBranch,
			Private:            repo.Private,
			HTMLURL:            repo.HTMLURL,
		})
	}
	return a.store.ReplaceGitHubRepositories(ctx, inst.ID, sync)
}

func (a *api) requirePrincipal(c *gin.Context) (store.UserWithOrg, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return store.UserWithOrg{}, false
	}
	if a.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return store.UserWithOrg{}, false
	}
	principal, err := a.store.GetUserWithOrgByUserID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return store.UserWithOrg{}, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return store.UserWithOrg{}, false
	}
	return principal, true
}

func (a *api) redirectGitHubError(c *gin.Context, code string) {
	c.Redirect(http.StatusFound, a.cfg.AppBaseURL+"/settings/github?github_error="+code)
}
