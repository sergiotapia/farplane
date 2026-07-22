package httpapi

import (
	"context"
	"encoding/json"
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
	Name           string `json:"name"`
	LaneTemplateID string `json:"lane_template_id"`
	AgentProvider  string `json:"agent_provider"`
}

type patchLaneRequest struct {
	Name          *string `json:"name"`
	AgentProvider *string `json:"agent_provider"`
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
		out = append(out, laneJSON(l))
	}
	c.JSON(http.StatusOK, gin.H{"lanes": out})
}

func (a *api) handleCreateLane(c *gin.Context) {
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
	var req createLaneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	provider := strings.TrimSpace(req.AgentProvider)
	if !agents.IsKnownProvider(provider) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_provider"})
		return
	}
	required := agents.RequiredSecretFor(provider)
	set, err := a.store.OrganizationSecretIsSet(c.Request.Context(), principal.Organization.ID, required)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check secrets"})
		return
	}
	if !set {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "required secret is not set for this agent; configure Settings → Secrets",
		})
		return
	}
	templateID := strings.TrimSpace(req.LaneTemplateID)
	if templateID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "lane_template_id is required"})
		return
	}
	tmpl, err := a.store.GetLaneTemplate(c.Request.Context(), templateID)
	if err != nil {
		writeStoreError(c, err)
		return
	}
	if tmpl.OrganizationID != principal.Organization.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if tmpl.ValidationStatus != models.LaneTemplateValidationValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "lane template is not valid; validate it first"})
		return
	}

	snapshot := tmpl.DockerfileText
	var imageRef *string
	if tmpl.ValidatedImageReference != nil && *tmpl.ValidatedImageReference != "" {
		imageRef = tmpl.ValidatedImageReference
	} else if a.runtime != nil {
		tag := "farplane-lane-" + project.ID[:8]
		ref, _, buildErr := a.runtime.BuildImage(c.Request.Context(), snapshot, tag)
		if buildErr != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "failed to build lane image", "detail": buildErr.Error()})
			return
		}
		imageRef = &ref
	}
	if imageRef == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template has no validated image"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Lane"
	}
	if utf8.RuneCountInString(name) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is too long"})
		return
	}

	lane, err := a.store.CreateLane(c.Request.Context(), store.CreateLaneInput{
		ProjectID:          project.ID,
		OrganizationID:     principal.Organization.ID,
		OwnerUserID:        principal.User.ID,
		Name:               name,
		LaneTemplateID:     &tmpl.ID,
		DockerfileSnapshot: snapshot,
		ImageReference:     imageRef,
		RuntimeKind:        models.RuntimeKindDocker,
		AgentProvider:      provider,
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
				"lane":    laneJSON(lane),
				"warning": "lane created but runtime failed to start: " + createErr.Error(),
			})
			return
		}
		_ = a.runtime.InjectSecrets(c.Request.Context(), inst.ID, secrets)
		_ = a.runtime.EnsureAgentBridge(c.Request.Context(), inst.ID)
		if cloneErr := a.cloneProjectIntoLane(c.Request.Context(), inst.ID, project); cloneErr != nil {
			log.Printf(
				"lane create git clone failed lane_id=%s project_id=%s err=%v",
				lane.ID,
				project.ID,
				cloneErr,
			)
		}
		runtimeID := inst.ID
		lane, _ = a.store.UpdateLaneRuntime(c.Request.Context(), lane.ID, &runtimeID, imageRef, models.LaneStatusRunning)
	}

	c.JSON(http.StatusCreated, laneJSON(lane))
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
	if _, err := a.store.RequireActiveLaneParticipant(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, laneJSON(lane))
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
	if _, err := a.store.RequireLaneOwner(c.Request.Context(), lane.ID, principal.User.ID); err != nil {
		writeStoreError(c, err)
		return
	}
	var req patchLaneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.AgentProvider != nil {
		if a.hub != nil && a.hub.IsTurnBusy(lane.ID) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot switch agent while a turn is running"})
			return
		}
		provider := strings.TrimSpace(*req.AgentProvider)
		if !agents.IsKnownProvider(provider) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_provider"})
			return
		}
		required := agents.RequiredSecretFor(provider)
		set, err := a.store.OrganizationSecretIsSet(c.Request.Context(), principal.Organization.ID, required)
		if err != nil || !set {
			c.JSON(http.StatusBadRequest, gin.H{"error": "required secret is not set for this agent"})
			return
		}
		from := lane.AgentProvider
		lane, err = a.store.UpdateLaneAgentProvider(c.Request.Context(), lane.ID, provider)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update agent"})
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"from":    from,
			"to":      provider,
			"user_id": principal.User.ID,
			"at":      time.Now().UTC(),
		})
		role := models.LaneMessageRoleSystem
		body := fmt.Sprintf("Agent switched from %s to %s", from, provider)
		msg, _ := a.store.InsertLaneMessage(c.Request.Context(), store.InsertLaneMessageInput{
			LaneID:    lane.ID,
			EventType: models.LaneEventAgentChanged,
			Role:      &role,
			Body:      &body,
			Payload:   payload,
		})
		if a.hub != nil {
			a.hub.BroadcastMessage(lane.ID, msg)
		}
		if a.runtime != nil && lane.RuntimeID != nil {
			transcript, _ := a.store.BuildHandoffTranscript(c.Request.Context(), lane.ID, 40)
			_ = a.runtime.SendUserTurn(c.Request.Context(), *lane.RuntimeID, runtime.UserTurn{
				Type:       "handoff",
				LaneID:     lane.ID,
				Provider:   provider,
				Transcript: transcript,
			})
		}
	}
	c.JSON(http.StatusOK, laneJSON(lane))
}

func laneJSON(l models.Lane) gin.H {
	return gin.H{
		"id":                        l.ID,
		"project_id":                l.ProjectID,
		"organization_id":           l.OrganizationID,
		"owner_user_id":             l.OwnerUserID,
		"name":                      l.Name,
		"lane_template_id":          l.LaneTemplateID,
		"image_reference":           l.ImageReference,
		"runtime_kind":              l.RuntimeKind,
		"runtime_id":                l.RuntimeID,
		"agent_provider":            l.AgentProvider,
		"agent_provider_session_id": l.AgentProviderSessionID,
		"status":                    l.Status,
		"created_at":                l.CreatedAt,
		"updated_at":                l.UpdatedAt,
	}
}
