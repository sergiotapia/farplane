package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type createLaneTemplateRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	DockerfileText string `json:"dockerfile_text"`
}

type updateLaneTemplateRequest struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	DockerfileText *string `json:"dockerfile_text"`
}

type forkLaneTemplateRequest struct {
	Name string `json:"name"`
}

func (a *api) handleListLaneTemplates(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	items, err := a.store.ListLaneTemplates(c.Request.Context(), principal.Organization.ID, principal.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list lane templates"})
		return
	}
	ids := make([]string, 0, len(items))
	for _, t := range items {
		ids = append(ids, t.ID)
	}
	inUse, err := a.store.LaneTemplatesInUse(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list lane templates"})
		return
	}
	out := make([]gin.H, 0, len(items))
	for _, t := range items {
		out = append(out, laneTemplateJSON(t, inUse[t.ID]))
	}
	c.JSON(http.StatusOK, gin.H{"lane_templates": out})
}

func (a *api) handleCreateLaneTemplate(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	var req createLaneTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	text := strings.TrimSpace(req.DockerfileText)
	if name == "" || text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and dockerfile_text are required"})
		return
	}
	if utf8.RuneCountInString(name) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is too long"})
		return
	}
	uid := principal.User.ID
	lintOK, lintLog := runDockerfileLint(c.Request.Context(), text)
	if !lintOK {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":               "dockerfile lint failed",
			"last_validation_log": lintLog,
		})
		return
	}
	t, err := a.store.CreateLaneTemplate(c.Request.Context(), store.CreateLaneTemplateInput{
		OrganizationID:    principal.Organization.ID,
		Name:              name,
		Description:       req.Description,
		DockerfileText:    text,
		CreatedByUserID:   &uid,
		LastValidationLog: &lintLog,
	})
	if err != nil {
		log.Printf(
			"lane template create failed organization_id=%s name=%q err=%v",
			principal.Organization.ID,
			name,
			err,
		)
		if strings.Contains(err.Error(), "template name already exists") {
			c.JSON(http.StatusConflict, gin.H{
				"error": "a template with this name already exists",
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, laneTemplateJSON(t, false))
}

func (a *api) handleGetLaneTemplate(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	t, err := a.store.GetLaneTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if t.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	inUse, err := a.store.LaneTemplatesInUse(c.Request.Context(), []string{t.ID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load lane template"})
		return
	}
	c.JSON(http.StatusOK, laneTemplateJSON(t, inUse[t.ID]))
}

func (a *api) handleUpdateLaneTemplate(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	t, err := a.store.GetLaneTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if t.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req updateLaneTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	in := store.UpdateLaneTemplateInput{
		Name:            req.Name,
		Description:     req.Description,
		DockerfileText:  req.DockerfileText,
		UpdatedByUserID: principal.User.ID,
	}
	if req.DockerfileText != nil {
		text := strings.TrimSpace(*req.DockerfileText)
		lintOK, lintLog := runDockerfileLint(c.Request.Context(), text)
		if !lintOK {
			// Do not persist a broken Dockerfile; return lint output for the editor.
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":               "dockerfile lint failed",
				"last_validation_log": lintLog,
			})
			return
		}
		in.LastValidationLog = &lintLog
	}
	updated, err := a.store.UpdateLaneTemplate(c.Request.Context(), t.ID, in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	inUse, err := a.store.LaneTemplatesInUse(c.Request.Context(), []string{updated.ID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update lane template"})
		return
	}
	c.JSON(http.StatusOK, laneTemplateJSON(updated, inUse[updated.ID]))
}

func (a *api) handleForkLaneTemplate(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	t, err := a.store.GetLaneTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if t.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req forkLaneTemplateRequest
	_ = c.ShouldBindJSON(&req)
	forked, err := a.store.ForkLaneTemplate(c.Request.Context(), t.ID, req.Name, principal.User.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, laneTemplateJSON(forked, false))
}

func (a *api) handleDeleteLaneTemplate(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	t, err := a.store.GetLaneTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if t.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err := a.store.DeleteLaneTemplate(c.Request.Context(), t.ID); err != nil {
		switch {
		case errors.Is(err, store.ErrLaneTemplateInUse):
			c.JSON(http.StatusConflict, gin.H{
				"error": "This template is used by a Lane, so it cannot be deleted.",
			})
		case errors.Is(err, store.ErrLaneTemplateIsDefault):
			c.JSON(http.StatusConflict, gin.H{
				"error": "The default template cannot be deleted.",
			})
		default:
			writeStoreError(c, err)
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) handleValidateLaneTemplate(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	t, err := a.store.GetLaneTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if t.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if a.runtime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "runtime unavailable"})
		return
	}
	tag := "farplane-template-" + t.ID[:8]
	imageRef, logText, buildErr := a.runtime.BuildImage(c.Request.Context(), t.DockerfileText, tag)
	okBuild := buildErr == nil
	if buildErr != nil && logText == "" {
		logText = buildErr.Error()
	}
	if buildErr != nil {
		log.Printf(
			"lane template validate failed template_id=%s tag=%s err=%v",
			t.ID,
			tag,
			buildErr,
		)
		if tail := validationLogTail(logText, 4000); tail != "" {
			log.Printf("lane template validate build log (tail) template_id=%s:\n%s", t.ID, tail)
		}
	} else {
		log.Printf(
			"lane template validate ok template_id=%s image_reference=%s",
			t.ID,
			imageRef,
		)
	}
	updated, err := a.store.CompleteLaneTemplateValidation(c.Request.Context(), t.ID, okBuild, imageRef, logText)
	if err != nil {
		log.Printf(
			"lane template validate save failed template_id=%s err=%v",
			t.ID,
			err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save validation result"})
		return
	}
	status := http.StatusOK
	if !okBuild {
		status = http.StatusUnprocessableEntity
	}
	inUse, inUseErr := a.store.LaneTemplatesInUse(c.Request.Context(), []string{updated.ID})
	if inUseErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save validation result"})
		return
	}
	c.JSON(status, laneTemplateJSON(updated, inUse[updated.ID]))
}

// validationLogTail returns the last maxLen runes of logText for server logs.
func validationLogTail(logText string, maxLen int) string {
	if logText == "" || maxLen <= 0 {
		return logText
	}
	runes := []rune(logText)
	if len(runes) <= maxLen {
		return logText
	}
	return string(runes[len(runes)-maxLen:])
}

func laneTemplateJSON(t models.LaneTemplate, inUse bool) gin.H {
	return gin.H{
		"id":                        t.ID,
		"organization_id":           t.OrganizationID,
		"name":                      t.Name,
		"description":               t.Description,
		"dockerfile_text":           t.DockerfileText,
		"is_system_default":         t.IsSystemDefault,
		"forked_from_template_id":   t.ForkedFromTemplateID,
		"created_by_user_id":        t.CreatedByUserID,
		"updated_by_user_id":        t.UpdatedByUserID,
		"validation_status":         t.ValidationStatus,
		"validated_image_reference": t.ValidatedImageReference,
		"last_validation_log":       t.LastValidationLog,
		"validated_at":              t.ValidatedAt,
		"created_at":                t.CreatedAt,
		"updated_at":                t.UpdatedAt,
		"in_use":                    inUse,
	}
}
