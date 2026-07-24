package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type createProjectRequest struct {
	Name               string `json:"name"`
	GitHubRepositoryID int64  `json:"github_repository_id"`
}

func (a *api) handleListProjects(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	projects, err := a.store.ListProjects(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to list projects"})
		return
	}

	items := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		items = append(items, projectJSON(p))
	}

	c.JSON(http.StatusOK, gin.H{"projects": items})
}

func (a *api) handleGetProject(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	project, err := a.store.GetProject(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}

	if project.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{jsonKeyError: errNotFound})
		return
	}

	c.JSON(http.StatusOK, projectJSON(project))
}

func (a *api) handleCreateProject(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: errInvalidRequestBody})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "name is required"})
		return
	}

	if utf8.RuneCountInString(name) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "name is too long"})
		return
	}

	if req.GitHubRepositoryID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "github_repository_id is required"})
		return
	}

	repo, err := a.store.GetPickableGitHubRepository(
		c.Request.Context(), principal.Organization.ID, req.GitHubRepositoryID,
	)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "github repository is not available"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to resolve repository"})

		return
	}

	project, err := a.store.CreateProject(c.Request.Context(), store.CreateProjectInput{
		OrganizationID:       principal.Organization.ID,
		Name:                 name,
		GitHubRepositoryID:   repo.GitHubRepositoryID,
		GitHubInstallationID: repo.GitHubInstallationID,
		DefaultBranch:        repo.DefaultBranch,
		GitHubFullName:       repo.FullName,
		CreatedByUserID:      principal.User.ID,
	})
	if err != nil {
		if errors.Is(err, store.ErrProjectRepoExists) {
			c.JSON(http.StatusConflict, gin.H{jsonKeyError: "project already exists for repository"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to create project"})

		return
	}

	c.JSON(http.StatusCreated, projectJSON(project))
}

func projectJSON(p models.Project) gin.H {
	return gin.H{
		"id":                     p.ID,
		jsonKeyOrganizationID:    p.OrganizationID,
		jsonKeyName:              p.Name,
		"github_repository_id":   p.GitHubRepositoryID,
		"github_installation_id": p.GitHubInstallationID,
		"default_branch":         p.DefaultBranch,
		"github_full_name":       p.GitHubFullName,
		"github_access_status":   p.GitHubAccessStatus,
		"created_by_user_id":     p.CreatedByUserID,
		jsonKeyCreatedAt:         p.CreatedAt,
		jsonKeyUpdatedAt:         p.UpdatedAt,
	}
}
