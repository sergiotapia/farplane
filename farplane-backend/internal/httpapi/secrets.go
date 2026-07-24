package httpapi

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
)

type setSecretRequest struct {
	Value string `json:"value"`
}

func (a *api) handleListSecrets(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	metas, err := a.store.ListOrganizationSecretMeta(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to list secrets"})
		return
	}

	out := make([]gin.H, 0, len(metas))
	for _, m := range metas {
		out = append(out, secretMetaJSON(m))
	}

	c.JSON(http.StatusOK, gin.H{"secrets": out})
}

func (a *api) handleSetSecret(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	name := strings.TrimSpace(c.Param("name"))
	if !isWellKnownSecret(name) {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "unknown secret name"})
		return
	}

	var req setSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Value) == "" {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "value is required"})
		return
	}

	value := strings.TrimSpace(req.Value)
	if err := a.store.SetOrganizationSecret(
		c.Request.Context(),
		principal.Organization.ID,
		name,
		value,
		a.cfg.SessionSecret,
		principal.User.ID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to set secret"})
		return
	}

	a.reinjectOrgSecrets(c, principal.Organization.ID)
	c.JSON(http.StatusOK, gin.H{
		jsonKeyName: name,
		"is_set":    true,
		"label":     agents.SecretLabel(name),
	})
}

// reinjectOrgSecrets pushes the latest decrypted secrets into running Lanes.
func (a *api) reinjectOrgSecrets(c *gin.Context, organizationID string) {
	if a.runtime == nil || a.store == nil {
		return
	}

	secrets, err := a.store.DecryptOrganizationSecrets(
		c.Request.Context(), organizationID, a.cfg.SessionSecret,
	)
	if err != nil || len(secrets) == 0 {
		return
	}

	lanes, err := a.store.ListRunningLanesForOrganization(c.Request.Context(), organizationID)
	if err != nil {
		return
	}

	for _, lane := range lanes {
		if lane.RuntimeID == nil || *lane.RuntimeID == "" {
			continue
		}

		_ = a.runtime.InjectSecrets(c.Request.Context(), *lane.RuntimeID, secrets)
	}
}

func (a *api) handleClearSecret(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	name := strings.TrimSpace(c.Param("name"))
	if !isWellKnownSecret(name) {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "unknown secret name"})
		return
	}

	if err := a.store.ClearOrganizationSecret(c.Request.Context(), principal.Organization.ID, name); err != nil {
		writeStoreError(c, err)
		return
	}

	a.reinjectOrgSecrets(c, principal.Organization.ID)
	c.Status(http.StatusNoContent)
}

func (a *api) handleListLaneAgents(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	set, err := a.store.SetSecretsMap(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to load secrets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents.Availability(set)})
}

func (a *api) handleListLaneAgentModels(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	provider := strings.TrimSpace(c.Param("provider"))
	if !agents.IsKnownProvider(provider) {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "invalid agent_provider"})
		return
	}

	source := strings.TrimSpace(c.Query("source"))
	if source == "" {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: "source query parameter is required"})
		return
	}

	setSecrets, err := a.store.SetSecretsMap(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{jsonKeyError: "failed to load secrets"})
		return
	}

	if !agents.SourceAllowedForAgent(provider, source, setSecrets) {
		c.JSON(http.StatusBadRequest, gin.H{jsonKeyError: errModelSource})
		return
	}

	catalog := a.agentCatalog()

	list, err := catalog.ModelsFor(c.Request.Context(), provider, source)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{jsonKeyError: "failed to load models for agent"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"models": list})
}

func isWellKnownSecret(name string) bool {
	return slices.Contains(agents.WellKnownSecretNames, name)
}

func secretMetaJSON(m models.OrganizationSecretMeta) gin.H {
	return gin.H{
		jsonKeyName:      m.Name,
		"label":          agents.SecretLabel(m.Name),
		"is_set":         m.IsSet,
		jsonKeyUpdatedAt: m.UpdatedAt,
	}
}
