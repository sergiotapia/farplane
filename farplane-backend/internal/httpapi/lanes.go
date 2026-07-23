package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/farplane/farplane/farplane-backend/internal/agents"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/runtime"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type createLaneRequest struct {
	Name            string  `json:"name"`
	AgentProvider   string  `json:"agent_provider"`
	ModelSource     *string `json:"model_source"`
	AgentModel      *string `json:"agent_model"`
	ReasoningEffort *string `json:"reasoning_effort"`
	ProjectID       *string `json:"project_id"`
}

type patchLaneRequest struct {
	Name            *string `json:"name"`
	AgentProvider   *string `json:"agent_provider"`
	ModelSource     *string `json:"model_source"`
	AgentModel      *string `json:"agent_model"`
	ReasoningEffort *string `json:"reasoning_effort"`
}

func (a *api) handleListLanes(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	grouped, err := a.store.ListLanesGrouped(
		c.Request.Context(), principal.Organization.ID, principal.User.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list lanes"})
		return
	}
	projects := make([]gin.H, 0, len(grouped.Projects))
	for _, g := range grouped.Projects {
		lanes := make([]gin.H, 0, len(g.Lanes))
		for _, l := range g.Lanes {
			lanes = append(lanes, a.laneJSON(l))
		}
		projects = append(projects, gin.H{
			"id":    g.ID,
			"name":  g.Name,
			"lanes": lanes,
		})
	}
	scratch := make([]gin.H, 0, len(grouped.ScratchLanes))
	for _, l := range grouped.ScratchLanes {
		scratch = append(scratch, a.laneJSON(l))
	}
	c.JSON(http.StatusOK, gin.H{
		"projects":      projects,
		"scratch_lanes": scratch,
	})
}

func (a *api) handleListProjectLanes(c *gin.Context) {
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
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	lanes, err := a.store.ListLanesForProject(c.Request.Context(), project.ID, principal.User.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list lanes"})
		return
	}
	out := make([]gin.H, 0, len(lanes))
	for _, l := range lanes {
		out = append(out, a.laneJSON(l))
	}
	c.JSON(http.StatusOK, gin.H{"lanes": out})
}

func (a *api) handleCreateLaneForProject(c *gin.Context) {
	var req createLaneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	projectID := c.Param("id")
	req.ProjectID = &projectID
	a.createLaneFromRequest(c, req)
}

func (a *api) handleCreateLaneTopLevel(c *gin.Context) {
	var req createLaneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	a.createLaneFromRequest(c, req)
}

func (a *api) createLaneFromRequest(c *gin.Context, req createLaneRequest) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}

	var project *models.Project
	var projectID *string
	if req.ProjectID != nil && strings.TrimSpace(*req.ProjectID) != "" {
		pid := strings.TrimSpace(*req.ProjectID)
		p, err := a.store.GetProject(c.Request.Context(), pid)
		if err != nil {
			writeStoreError(c, err)
			return
		}
		if p.OrganizationID != principal.Organization.ID {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		project = &p
		projectID = &p.ID
	}

	provider := strings.TrimSpace(req.AgentProvider)
	if !agents.IsKnownProvider(provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_provider"})
		return
	}
	setSecrets, err := a.store.SetSecretsMap(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check secrets"})
		return
	}
	if !agents.AgentAvailable(provider, setSecrets) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "required secret is not set for this agent; configure Settings → Secrets",
		})
		return
	}
	selection, err := a.resolveCreateModelSelection(
		c.Request.Context(), provider, setSecrets, req.ModelSource, req.AgentModel, req.ReasoningEffort,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": clientErrorForModelSelection(err)})
		return
	}
	var snapshot string
	var imageRef *string
	if projectID != nil {
		env, envErr := a.store.GetProjectEnvironment(c.Request.Context(), *projectID)
		if envErr != nil {
			if errors.Is(envErr, store.ErrNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "project has no Project Environment; generate or save one first",
				})
				return
			}
			writeStoreError(c, envErr)
			return
		}
		if env.ValidationStatus != models.EnvironmentValidationValid {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "project environment is not valid; validate it first",
			})
			return
		}
		snapshot = env.DockerfileText
		imageRef = env.ValidatedImageReference
	} else {
		env, envErr := a.store.EnsureScratchEnvironment(
			c.Request.Context(), principal.Organization.ID, principal.User.ID,
		)
		if envErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load scratch environment"})
			return
		}
		if env.ValidationStatus != models.EnvironmentValidationValid {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "scratch environment is not valid; validate it first",
			})
			return
		}
		snapshot = env.DockerfileText
		imageRef = env.ValidatedImageReference
	}
	if imageRef == nil || *imageRef == "" {
		if a.runtime == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "environment has no validated image"})
			return
		}
		tagSuffix := "scratch"
		if projectID != nil && len(*projectID) >= 8 {
			tagSuffix = (*projectID)[:8]
		}
		tag := "farplane-lane-" + tagSuffix
		ref, _, buildErr := a.runtime.BuildImage(c.Request.Context(), snapshot, tag)
		if buildErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to build lane image", "detail": buildErr.Error()})
			return
		}
		imageRef = &ref
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Lane"
	}
	if utf8.RuneCountInString(name) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is too long"})
		return
	}

	kind := models.LaneKindScratch
	if projectID != nil {
		kind = models.LaneKindProject
	}

	lane, err := a.store.CreateLane(c.Request.Context(), store.CreateLaneInput{
		ProjectID:          projectID,
		OrganizationID:     principal.Organization.ID,
		OwnerUserID:        principal.User.ID,
		Name:               name,
		LaneKind:           kind,
		DockerfileSnapshot: snapshot,
		ImageReference:     imageRef,
		RuntimeKind:        models.RuntimeKindDocker,
		AgentProvider:      provider,
		ModelSource:        selection.ModelSource,
		AgentModel:         selection.AgentModel,
		ReasoningEffort:    selection.ReasoningEffort,
		Status:             models.LaneStatusCreating,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create lane"})
		return
	}

	if a.runtime != nil {
		secrets, secErr := a.store.DecryptOrganizationSecrets(
			c.Request.Context(), principal.Organization.ID, a.cfg.SessionSecret,
		)
		if secErr != nil {
			_, _ = a.store.UpdateLaneRuntime(c.Request.Context(), lane.ID, nil, imageRef, models.LaneStatusError)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load secrets for lane"})
			return
		}
		env := map[string]string{
			"FARPLANE_AGENT_PROVIDER": provider,
		}
		for k, v := range secrets {
			env[k] = v
		}
		inst, createErr := a.runtime.Create(c.Request.Context(), runtime.CreateRequest{
			LaneID:         lane.ID,
			ImageReference: *imageRef,
			Name:           "farplane-lane-" + lane.ID[:8],
			Env:            env,
			Labels: map[string]string{
				"farplane.lane_id": lane.ID,
			},
		})
		if createErr != nil {
			_, _ = a.store.UpdateLaneRuntime(c.Request.Context(), lane.ID, nil, imageRef, models.LaneStatusError)
			c.JSON(http.StatusCreated, gin.H{
				"lane":    a.laneJSON(lane),
				"warning": "lane created but runtime failed to start: " + createErr.Error(),
			})
			return
		}
		_ = a.runtime.InjectSecrets(c.Request.Context(), inst.ID, secrets)
		_ = a.runtime.EnsureAgentBridge(c.Request.Context(), inst.ID)
		if kind == models.LaneKindProject && project != nil {
			if cloneErr := a.cloneProjectIntoLane(c.Request.Context(), inst.ID, *project); cloneErr != nil {
				log.Printf(
					"lane create git clone failed lane_id=%s project_id=%s err=%v",
					lane.ID,
					project.ID,
					cloneErr,
				)
			}
		}
		runtimeID := inst.ID
		lane, _ = a.store.UpdateLaneRuntime(c.Request.Context(), lane.ID, &runtimeID, imageRef, models.LaneStatusRunning)
	}

	c.JSON(http.StatusCreated, a.laneJSON(lane))
}

// cloneProjectIntoLane checks out the Project GitHub repo into /workspace.
func (a *api) cloneProjectIntoLane(ctx context.Context, runtimeID string, project models.Project) error {
	gh := a.githubApp()
	if gh == nil {
		return fmt.Errorf("github app is not configured")
	}
	inst, err := a.store.GetGitHubInstallationByID(ctx, project.GitHubInstallationID)
	if err != nil {
		return err
	}
	token, _, err := gh.CreateInstallationToken(ctx, inst.GitHubInstallationID)
	if err != nil {
		return err
	}
	branch := strings.TrimSpace(project.DefaultBranch)
	if branch == "" {
		branch = "main"
	}
	repo := strings.TrimSpace(project.GitHubFullName)
	if repo == "" {
		return fmt.Errorf("project has no github_full_name")
	}
	script := `set -e
git clone --depth 1 --branch "$FARPLANE_GIT_BRANCH" \
  "https://x-access-token:${FARPLANE_GIT_TOKEN}@github.com/${FARPLANE_GIT_REPO}.git" \
  /tmp/farplane-repo
cp -a /tmp/farplane-repo/. /workspace/
rm -rf /tmp/farplane-repo`
	sess, err := a.runtime.Exec(ctx, runtimeID, runtime.ExecRequest{
		Command: []string{"sh", "-c", script},
		Env: map[string]string{
			"FARPLANE_GIT_TOKEN":  token,
			"FARPLANE_GIT_BRANCH": branch,
			"FARPLANE_GIT_REPO":   repo,
		},
		WorkDir: "/workspace",
	})
	if err != nil {
		return err
	}
	defer sess.Close()
	code, err := sess.Wait()
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("git clone exited with code %d", code)
	}
	return nil
}

func (a *api) handleGetLane(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if lane.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if lane.Status == models.LaneStatusDestroyed {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, err := a.store.RequireActiveLaneParticipant(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, a.laneJSON(lane))
}

func (a *api) handleDestroyLane(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if lane.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if lane.Status == models.LaneStatusDestroyed {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, err := a.store.RequireLaneOwner(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	if a.runtime != nil && lane.RuntimeID != nil && *lane.RuntimeID != "" {
		if err := a.runtime.Destroy(c.Request.Context(), *lane.RuntimeID); err != nil {
			log.Printf("lane destroy runtime failed lane_id=%s runtime_id=%s err=%v", lane.ID, *lane.RuntimeID, err)
		}
	}
	if _, err := a.store.DestroyLane(c.Request.Context(), lane.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) handlePatchLane(c *gin.Context) {
	principal, ok := a.requirePrincipal(c)
	if !ok {
		return
	}
	lane, err := a.store.GetLane(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if lane.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if lane.Status == models.LaneStatusDestroyed {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if _, err := a.store.RequireLaneOwner(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	var req patchLaneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	agentTouch := req.AgentProvider != nil ||
		req.ModelSource != nil ||
		req.AgentModel != nil ||
		req.ReasoningEffort != nil
	if !agentTouch {
		c.JSON(http.StatusOK, a.laneJSON(lane))
		return
	}
	if a.hub != nil && a.hub.IsTurnBusy(lane.ID) {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot change agent settings while a turn is running"})
		return
	}

	setSecrets, err := a.store.SetSecretsMap(c.Request.Context(), principal.Organization.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check secrets"})
		return
	}

	provider := lane.AgentProvider
	providerChanged := false
	if req.AgentProvider != nil {
		provider = strings.TrimSpace(*req.AgentProvider)
		if !agents.IsKnownProvider(provider) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_provider"})
			return
		}
		if !agents.AgentAvailable(provider, setSecrets) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "required secret is not set for this agent"})
			return
		}
		providerChanged = provider != lane.AgentProvider
	}

	source := lane.ModelSource
	sourceChanged := false
	if req.ModelSource != nil {
		source = strings.TrimSpace(*req.ModelSource)
		sourceChanged = source != lane.ModelSource
	}

	catalog := a.agentCatalog()
	model, effort := mergeRequestedModelSelection(lane, req)

	var selection agents.ModelSelection
	var selErr error
	switch {
	case providerChanged:
		selection, selErr = catalog.ResolveForAgentChange(
			c.Request.Context(), provider, source, model, effort, setSecrets,
		)
	case sourceChanged:
		if !agents.SourceAllowedForAgent(provider, source, setSecrets) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model_source not available for this agent"})
			return
		}
		selection, selErr = catalog.ResolveForSourceChange(
			c.Request.Context(), provider, source, model, effort,
		)
	case req.AgentModel != nil || req.ReasoningEffort != nil:
		if !agents.SourceAllowedForAgent(provider, source, setSecrets) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "model_source not available for this agent"})
			return
		}
		selection, selErr = catalog.ValidateSelection(
			c.Request.Context(), provider, source, model, effort,
		)
	default:
		selection = agents.ModelSelection{
			ModelSource:     lane.ModelSource,
			AgentModel:      lane.AgentModel,
			ReasoningEffort: lane.ReasoningEffort,
		}
	}
	if selErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": clientErrorForModelSelection(selErr)})
		return
	}

	from := lane.AgentProvider
	lane, err = a.store.UpdateLaneAgentSettings(c.Request.Context(), lane.ID, store.UpdateLaneAgentSettingsInput{
		AgentProvider:   provider,
		ModelSource:     selection.ModelSource,
		AgentModel:      selection.AgentModel,
		ReasoningEffort: selection.ReasoningEffort,
		ClearSession:    providerChanged,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update lane"})
		return
	}

	if providerChanged {
		payload, err := json.Marshal(map[string]any{
			"from":             from,
			"to":               provider,
			"model_source":     lane.ModelSource,
			"agent_model":      lane.AgentModel,
			"reasoning_effort": lane.ReasoningEffort,
			"user_id":          principal.User.ID,
			"at":               time.Now().UTC(),
		})
		if err != nil {
			log.Printf("lane agent switch payload marshal failed lane_id=%s err=%v", lane.ID, err)
			payload = []byte("{}")
		}
		role := models.LaneMessageRoleSystem
		body := fmt.Sprintf("Agent switched from %s to %s", from, provider)
		msg, err := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
			LaneID:    lane.ID,
			EventType: models.LaneEventAgentChanged,
			Role:      &role,
			Body:      &body,
			Payload:   payload,
		})
		if err != nil {
			log.Printf("lane agent switch message insert failed lane_id=%s err=%v", lane.ID, err)
		} else if a.hub != nil {
			a.hub.BroadcastMessage(lane.ID, msg)
		}
		if a.runtime != nil && lane.RuntimeID != nil {
			transcript, err := a.store.BuildHandoffTranscript(c.Request.Context(), lane.ID, 40)
			if err != nil {
				log.Printf("lane agent switch transcript failed lane_id=%s err=%v", lane.ID, err)
			} else if err := a.runtime.SendUserTurn(c.Request.Context(), *lane.RuntimeID, runtime.UserTurn{
				Type:       "handoff",
				LaneID:     lane.ID,
				Provider:   provider,
				Transcript: transcript,
			}); err != nil {
				log.Printf("lane agent switch handoff failed lane_id=%s runtime_id=%s err=%v", lane.ID, *lane.RuntimeID, err)
			}
		}
	}
	c.JSON(http.StatusOK, a.laneJSON(lane))
}

func mergeRequestedModelSelection(lane models.Lane, req patchLaneRequest) (string, *string) {
	model := lane.AgentModel
	effort := lane.ReasoningEffort
	if req.AgentModel != nil {
		model = strings.TrimSpace(*req.AgentModel)
	}
	if req.ReasoningEffort != nil {
		effort = req.ReasoningEffort
	}
	return model, effort
}

// clientErrorForModelSelection maps catalog errors to stable client messages.
func clientErrorForModelSelection(err error) string {
	if err == nil {
		return "invalid agent settings"
	}
	switch err.Error() {
	case "unknown agent provider",
		"unknown model_source",
		"model_source not supported for this agent",
		"model_source not available for this agent",
		"no model source available for agent",
		"no models available for model_source",
		"model_source is required",
		"agent_model is required",
		"unknown agent_model for model_source",
		"invalid reasoning_effort for this model":
		return err.Error()
	default:
		log.Printf("model selection error: %v", err)
		return "invalid agent settings"
	}
}

func (a *api) resolveCreateModelSelection(
	ctx context.Context,
	provider string,
	setSecrets map[string]bool,
	modelSource, agentModel, reasoningEffort *string,
) (agents.ModelSelection, error) {
	catalog := a.agentCatalog()
	source := ""
	if modelSource != nil {
		source = strings.TrimSpace(*modelSource)
	}
	if source == "" {
		if agentModel == nil || strings.TrimSpace(*agentModel) == "" {
			return catalog.DefaultSelection(ctx, provider, setSecrets)
		}
		var ok bool
		source, ok = agents.DefaultModelSource(provider, setSecrets)
		if !ok {
			return agents.ModelSelection{}, fmt.Errorf("no model source available for agent")
		}
	}
	if !agents.SourceAllowedForAgent(provider, source, setSecrets) {
		return agents.ModelSelection{}, fmt.Errorf("model_source not available for this agent")
	}
	if agentModel == nil || strings.TrimSpace(*agentModel) == "" {
		return catalog.DefaultSelectionForSource(ctx, provider, source)
	}
	return catalog.ValidateSelection(ctx, provider, source, *agentModel, reasoningEffort)
}

func (a *api) laneJSON(l models.Lane) gin.H {
	turnRunning := false
	if a.hub != nil {
		turnRunning = a.hub.IsTurnBusy(l.ID)
	}
	out := gin.H{
		"id":                        l.ID,
		"project_id":                l.ProjectID,
		"organization_id":           l.OrganizationID,
		"owner_user_id":             l.OwnerUserID,
		"name":                      l.Name,
		"lane_kind":                 l.LaneKind,
		"image_reference":           l.ImageReference,
		"runtime_kind":              l.RuntimeKind,
		"runtime_id":                l.RuntimeID,
		"agent_provider":            l.AgentProvider,
		"agent_provider_session_id": l.AgentProviderSessionID,
		"model_source":              l.ModelSource,
		"agent_model":               l.AgentModel,
		"reasoning_effort":          l.ReasoningEffort,
		"status":                    l.Status,
		"turn_running":              turnRunning,
		"created_at":                l.CreatedAt,
		"updated_at":                l.UpdatedAt,
	}
	if l.HasOtherParticipants {
		out["has_other_participants"] = true
	} else {
		out["has_other_participants"] = false
	}
	return out
}
