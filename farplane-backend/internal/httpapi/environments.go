package httpapi

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/envgen"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type upsertEnvironmentRequest struct {
	DockerfileText string `json:"dockerfile_text"`
}

func (a *api) handleGetScratchEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	env, err := a.store.EnsureScratchEnvironment(
		c.Request.Context(), principal.Organization.ID, principal.User.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load scratch environment"})
		return
	}
	c.JSON(http.StatusOK, scratchEnvironmentJSON(env))
}

func (a *api) handleUpsertScratchEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	var req upsertEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	text := strings.TrimSpace(req.DockerfileText)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dockerfile_text is required"})
		return
	}
	if utf8.RuneCountInString(text) > 500_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dockerfile_text is too long"})
		return
	}
	lintOK, lintLog := runDockerfileLint(c.Request.Context(), text)
	if !lintOK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":               "dockerfile lint failed",
			"last_validation_log": lintLog,
		})
		return
	}
	env, err := a.store.UpsertScratchEnvironment(c.Request.Context(), store.UpsertScratchEnvironmentInput{
		OrganizationID:    principal.Organization.ID,
		DockerfileText:    text,
		UpdatedByUserID:   principal.User.ID,
		LastValidationLog: &lintLog,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, scratchEnvironmentJSON(env))
}

func (a *api) handleValidateScratchEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	env, err := a.store.EnsureScratchEnvironment(
		c.Request.Context(), principal.Organization.ID, principal.User.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load scratch environment"})
		return
	}
	if a.runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "runtime unavailable"})
		return
	}
	tag := "farplane-scratch-" + principal.Organization.ID[:8]
	imageRef, logText, buildErr := a.runtime.BuildImage(c.Request.Context(), env.DockerfileText, tag)
	okBuild := buildErr == nil
	if buildErr != nil && logText == "" {
		logText = buildErr.Error()
	}
	if buildErr != nil {
		log.Printf("scratch environment validate failed organization_id=%s err=%v", principal.Organization.ID, buildErr)
	}
	updated, err := a.store.CompleteScratchEnvironmentValidation(
		c.Request.Context(), principal.Organization.ID, okBuild, imageRef, logText,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save validation result"})
		return
	}
	status := http.StatusOK
	if !okBuild {
		status = http.StatusUnprocessableEntity
	}
	c.JSON(status, scratchEnvironmentJSON(updated))
}

func (a *api) handleGetProjectEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	project, ok := a.loadOrgProject(c, principal.Organization.ID, c.Param("id"))
	if !ok {
		return
	}
	env, err := a.store.GetProjectEnvironment(c.Request.Context(), project.ID)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, projectEnvironmentJSON(env))
}

func (a *api) handleUpsertProjectEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	project, ok := a.loadOrgProject(c, principal.Organization.ID, c.Param("id"))
	if !ok {
		return
	}
	var req upsertEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	text := strings.TrimSpace(req.DockerfileText)
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dockerfile_text is required"})
		return
	}
	if utf8.RuneCountInString(text) > 500_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dockerfile_text is too long"})
		return
	}
	lintOK, lintLog := runDockerfileLint(c.Request.Context(), text)
	if !lintOK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":               "dockerfile lint failed",
			"last_validation_log": lintLog,
		})
		return
	}
	env, err := a.store.UpsertProjectEnvironment(c.Request.Context(), store.UpsertProjectEnvironmentInput{
		ProjectID:         project.ID,
		OrganizationID:    principal.Organization.ID,
		DockerfileText:    text,
		UpdatedByUserID:   principal.User.ID,
		LastValidationLog: &lintLog,
		GenerationStatus:  models.EnvironmentGenerationIdle,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, projectEnvironmentJSON(env))
}

func (a *api) handleValidateProjectEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	project, ok := a.loadOrgProject(c, principal.Organization.ID, c.Param("id"))
	if !ok {
		return
	}
	env, err := a.store.GetProjectEnvironment(c.Request.Context(), project.ID)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if a.runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "runtime unavailable"})
		return
	}
	tag := "farplane-project-" + project.ID[:8]
	imageRef, logText, buildErr := a.runtime.BuildImage(c.Request.Context(), env.DockerfileText, tag)
	okBuild := buildErr == nil
	if buildErr != nil && logText == "" {
		logText = buildErr.Error()
	}
	updated, err := a.store.CompleteProjectEnvironmentValidation(
		c.Request.Context(), project.ID, okBuild, imageRef, logText,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save validation result"})
		return
	}
	status := http.StatusOK
	if !okBuild {
		status = http.StatusUnprocessableEntity
	}
	c.JSON(status, projectEnvironmentJSON(updated))
}

func (a *api) handleGenerateProjectEnvironment(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	project, ok := a.loadOrgProject(c, principal.Organization.ID, c.Param("id"))
	if !ok {
		return
	}

	started := time.Now()
	log.Printf(
		"project environment generate start project_id=%s repo=%s organization_id=%s",
		project.ID, project.GitHubFullName, principal.Organization.ID,
	)

	if _, err := a.store.MarkProjectEnvironmentGenerating(
		c.Request.Context(), project.ID, principal.Organization.ID, principal.User.ID,
	); err != nil {
		log.Printf("project environment generate mark failed project_id=%s err=%v", project.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start generation"})
		return
	}

	secrets, err := a.store.DecryptOrganizationSecrets(
		c.Request.Context(), principal.Organization.ID, a.cfg.SessionSecret,
	)
	if err != nil {
		log.Printf("project environment generate secrets failed project_id=%s err=%v", project.ID, err)
		_, _ = a.store.CompleteProjectEnvironmentGeneration(
			c.Request.Context(), project.ID, false, "", "failed to load secrets: "+err.Error(), principal.User.ID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load secrets"})
		return
	}

	log.Printf("project environment generate cloning project_id=%s repo=%s", project.ID, project.GitHubFullName)
	workspace, cleanup, cloneErr := a.cloneProjectForGeneration(c, project)
	if cloneErr != nil {
		log.Printf("project environment generate clone failed project_id=%s err=%v", project.ID, cloneErr)
		env, _ := a.store.CompleteProjectEnvironmentGeneration(
			c.Request.Context(), project.ID, false, "", "clone failed: "+cloneErr.Error(), principal.User.ID,
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":               "failed to clone repository for discovery",
			"project_environment": projectEnvironmentJSON(env),
		})
		return
	}
	if cleanup != nil {
		defer cleanup()
	}
	log.Printf(
		"project environment generate clone ok project_id=%s workspace=%s",
		project.ID, workspace,
	)

	gen := a.environmentGenerator()
	result, genErr := gen.Generate(c.Request.Context(), envgen.Request{
		WorkspaceDir: workspace,
		RepoFullName: project.GitHubFullName,
		Secrets:      secrets,
	})
	if genErr != nil {
		log.Printf(
			"project environment generate failed project_id=%s elapsed=%s err=%v",
			project.ID, time.Since(started).Round(time.Millisecond), genErr,
		)
		generationLog := result.Log
		if generationLog != "" {
			generationLog += "\n"
		}
		generationLog += genErr.Error()
		env, _ := a.store.CompleteProjectEnvironmentGeneration(
			c.Request.Context(), project.ID, false, "", generationLog, principal.User.ID,
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":               "environment generation failed",
			"project_environment": projectEnvironmentJSON(env),
		})
		return
	}

	lintOK, lintLog := runDockerfileLint(c.Request.Context(), result.DockerfileText)
	generationLog := result.Log
	if !lintOK {
		log.Printf("project environment generate lint failed project_id=%s", project.ID)
		generationLog += "\nDockerfile lint failed after generation:\n" + lintLog
		env, _ := a.store.CompleteProjectEnvironmentGeneration(
			c.Request.Context(), project.ID, false, "", generationLog, principal.User.ID,
		)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":               "generated dockerfile failed lint",
			"last_validation_log": lintLog,
			"project_environment": projectEnvironmentJSON(env),
		})
		return
	}

	if lintLog != "" {
		generationLog += "\nLint OK:\n" + lintLog
	}
	env, err := a.store.CompleteProjectEnvironmentGeneration(
		c.Request.Context(), project.ID, true, result.DockerfileText, generationLog, principal.User.ID,
	)
	if err != nil {
		log.Printf("project environment generate save failed project_id=%s err=%v", project.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save generated environment"})
		return
	}
	// Generate already proved the image builds; mark valid with the built reference.
	if imageRef := strings.TrimSpace(result.ImageReference); imageRef != "" {
		validationLog := result.BuildLog
		if validationLog == "" {
			validationLog = "Validated during environment generation"
		}
		env, err = a.store.CompleteProjectEnvironmentValidation(
			c.Request.Context(), project.ID, true, imageRef, validationLog,
		)
		if err != nil {
			log.Printf("project environment generate mark valid failed project_id=%s err=%v", project.ID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark environment valid"})
			return
		}
	}
	log.Printf(
		"project environment generate ok project_id=%s elapsed=%s image=%s",
		project.ID, time.Since(started).Round(time.Millisecond), result.ImageReference,
	)
	c.JSON(http.StatusOK, projectEnvironmentJSON(env))
}

func (a *api) cloneProjectForGeneration(c *gin.Context, project models.Project) (string, func(), error) {
	if a.cloneProjectWorkspace != nil {
		return a.cloneProjectWorkspace(c.Request.Context(), project)
	}
	gh := a.githubApp()
	if gh == nil {
		return "", nil, errGitHubAppNotConfigured
	}
	inst, err := a.store.GetGitHubInstallationByID(c.Request.Context(), project.GitHubInstallationID)
	if err != nil {
		return "", nil, err
	}
	token, _, err := gh.CreateInstallationToken(c.Request.Context(), inst.GitHubInstallationID)
	if err != nil {
		return "", nil, err
	}
	branch := strings.TrimSpace(project.DefaultBranch)
	if branch == "" {
		branch = "main"
	}
	dir, err := envgen.CloneRepository(c.Request.Context(), project.GitHubFullName, branch, token)
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

var errGitHubAppNotConfigured = errString("github app is not configured")

type errString string

func (e errString) Error() string { return string(e) }

func (a *api) loadOrgProject(c *gin.Context, organizationID, projectID string) (models.Project, bool) {
	project, err := a.store.GetProject(c.Request.Context(), projectID)
	if err != nil {
		writeStoreError(c, err)
		return models.Project{}, false
	}
	if project.OrganizationID != organizationID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return models.Project{}, false
	}
	return project, true
}

func (a *api) environmentGenerator() envgen.Generator {
	if a.envGenerator != nil {
		return a.envGenerator
	}
	svc := envgen.New()
	if a.runtime != nil {
		svc.BuildImage = a.runtime.BuildImage
	}
	return svc
}

func scratchEnvironmentJSON(env models.ScratchEnvironment) gin.H {
	return gin.H{
		"organization_id":           env.OrganizationID,
		"dockerfile_text":           env.DockerfileText,
		"validation_status":         env.ValidationStatus,
		"validated_image_reference": env.ValidatedImageReference,
		"last_validation_log":       env.LastValidationLog,
		"validated_at":              env.ValidatedAt,
		"updated_by_user_id":        env.UpdatedByUserID,
		"created_at":                env.CreatedAt,
		"updated_at":                env.UpdatedAt,
	}
}

func projectEnvironmentJSON(env models.ProjectEnvironment) gin.H {
	return gin.H{
		"project_id":                env.ProjectID,
		"organization_id":           env.OrganizationID,
		"dockerfile_text":           env.DockerfileText,
		"validation_status":         env.ValidationStatus,
		"validated_image_reference": env.ValidatedImageReference,
		"last_validation_log":       env.LastValidationLog,
		"validated_at":              env.ValidatedAt,
		"generation_status":         env.GenerationStatus,
		"generation_log":            env.GenerationLog,
		"updated_by_user_id":        env.UpdatedByUserID,
		"created_at":                env.CreatedAt,
		"updated_at":                env.UpdatedAt,
	}
}
