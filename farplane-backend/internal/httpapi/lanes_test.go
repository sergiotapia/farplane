package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/runtime"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type fakeRuntime struct {
	mu        sync.Mutex
	created   []runtime.CreateRequest
	destroyed []string
	execs     int
}

func (f *fakeRuntime) Create(ctx context.Context, req runtime.CreateRequest) (runtime.Instance, error) {
	_ = ctx

	f.mu.Lock()
	defer f.mu.Unlock()

	f.created = append(f.created, req)

	return runtime.Instance{ID: "ctr-" + req.LaneID[:8], Status: "running"}, nil
}

func (f *fakeRuntime) Destroy(ctx context.Context, id string) error {
	_ = ctx

	f.mu.Lock()
	defer f.mu.Unlock()

	f.destroyed = append(f.destroyed, id)

	return nil
}

func (f *fakeRuntime) Start(ctx context.Context, id string) error { _, _ = ctx, id; return nil }
func (f *fakeRuntime) Stop(ctx context.Context, id string) error  { _, _ = ctx, id; return nil }

func (f *fakeRuntime) Exec(ctx context.Context, id string, cmd runtime.ExecRequest) (runtime.ExecSession, error) {
	_, _, _ = ctx, id, cmd

	f.mu.Lock()
	f.execs++
	f.mu.Unlock()

	return &fakeExecSession{}, nil
}

func (f *fakeRuntime) InjectSecrets(ctx context.Context, id string, secrets map[string]string) error {
	_, _, _ = ctx, id, secrets
	return nil
}

func (f *fakeRuntime) PreviewURL(ctx context.Context, id string, port int) (string, error) {
	_, _, _ = ctx, id, port
	return "", nil
}

func (f *fakeRuntime) EnsureAgentBridge(ctx context.Context, id string) error {
	_, _ = ctx, id
	return nil
}

func (f *fakeRuntime) OpenAgentStream(ctx context.Context, id string) (runtime.AgentStream, error) {
	_, _ = ctx, id
	return nil, nil
}

func (f *fakeRuntime) SendUserTurn(ctx context.Context, id string, turn runtime.UserTurn) error {
	_, _, _ = ctx, id, turn
	return nil
}

func (f *fakeRuntime) InterruptTurn(ctx context.Context, id string) error { _, _ = ctx, id; return nil }

func (f *fakeRuntime) BuildImage(ctx context.Context, dockerfileText, tag string) (string, string, error) {
	_, _, _ = ctx, dockerfileText, tag
	return "farplane/test:latest", "ok", nil
}

type fakeExecSession struct{}

func (f *fakeExecSession) Wait() (int, error) { return 0, nil }
func (f *fakeExecSession) Stdout() io.Reader  { return strings.NewReader("") }
func (f *fakeExecSession) Stderr() io.Reader  { return strings.NewReader("") }
func (f *fakeExecSession) Close() error       { return nil }

func setupLaneTest(t *testing.T) (
	http.Handler,
	*http.Cookie,
	*store.Store,
	*fakeRuntime,
	testPrincipal,
	*pgxpool.Pool,
) {
	t.Helper()
	pool := openMigratedTestDB(t, true)
	st := store.New(pool)
	rt := &fakeRuntime{}
	engine := httpapi.New(pool, testConfig(), httpapi.WithRuntime(rt))

	rec := postSetup(engine, map[string]string{
		"organization_name": "Acme",
		"email":             "owner@example.com",
		"display_name":      "Owner",
		"password":          "password1",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d body=%s", rec.Code, rec.Body.String())
	}

	cookie := loginCookie(t, engine, "owner@example.com", "password1")
	principal := mustPrincipal(t, engine)

	if _, err := st.EnsureScratchEnvironment(context.Background(), principal.orgID, principal.userID); err != nil {
		t.Fatalf("EnsureScratchEnvironment: %v", err)
	}

	if _, err := st.CompleteScratchEnvironmentValidation(
		context.Background(), principal.orgID, true, "farplane/test:latest", "ok",
	); err != nil {
		t.Fatalf("CompleteScratchEnvironmentValidation: %v", err)
	}

	if err := st.SetOrganizationSecret(
		context.Background(),
		principal.orgID,
		models.SecretNameAnthropicAPIKey,
		"sk-test",
		testConfig().SessionSecret,
		principal.userID,
	); err != nil {
		t.Fatalf("SetOrganizationSecret: %v", err)
	}

	return engine, cookie, st, rt, principal, pool
}

func seedProject(t *testing.T, st *store.Store, principal testPrincipal) models.Project {
	t.Helper()

	inst, err := st.UpsertGitHubInstallation(context.Background(), store.UpsertGitHubInstallationInput{
		OrganizationID:       principal.orgID,
		GitHubInstallationID: 99,
		GitHubAccountID:      1,
		GitHubAccountLogin:   "alice",
		GitHubAccountType:    models.GitHubAccountTypeUser,
		RepositorySelection:  models.GitHubRepositorySelectionAll,
		ConnectedByUserID:    principal.userID,
	})
	if err != nil {
		t.Fatalf("UpsertGitHubInstallation: %v", err)
	}

	if err := st.ReplaceGitHubRepositories(context.Background(), inst.ID, []store.GitHubRepoSync{
		{
			GitHubRepositoryID: 55, FullName: "alice/app", DefaultBranch: "main",
			Private: true, HTMLURL: "https://github.com/alice/app",
		},
	}); err != nil {
		t.Fatalf("ReplaceGitHubRepositories: %v", err)
	}

	project, err := st.CreateProject(context.Background(), store.CreateProjectInput{
		OrganizationID:       principal.orgID,
		Name:                 "App",
		GitHubRepositoryID:   55,
		GitHubInstallationID: inst.ID,
		DefaultBranch:        "main",
		GitHubFullName:       "alice/app",
		CreatedByUserID:      principal.userID,
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	return project
}

func seedValidProjectEnvironment(t *testing.T, st *store.Store, project models.Project, userID string) {
	t.Helper()

	env, err := st.UpsertProjectEnvironment(context.Background(), store.UpsertProjectEnvironmentInput{
		ProjectID:        project.ID,
		OrganizationID:   project.OrganizationID,
		DockerfileText:   "FROM debian:bookworm-slim\n",
		UpdatedByUserID:  userID,
		GenerationStatus: models.EnvironmentGenerationIdle,
	})
	if err != nil {
		t.Fatalf("UpsertProjectEnvironment: %v", err)
	}

	if _, err := st.CompleteProjectEnvironmentValidation(
		context.Background(), env.ProjectID, true, "farplane/test:latest", "ok",
	); err != nil {
		t.Fatalf("CompleteProjectEnvironmentValidation: %v", err)
	}
}

func createLaneHTTP(
	t *testing.T,
	engine http.Handler,
	cookie *http.Cookie,
	body map[string]any,
) map[string]any {
	t.Helper()

	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lanes", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create lane = %d body=%s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode lane: %v", err)
	}

	return out
}

func TestCreateScratchAndProjectLaneKinds(t *testing.T) {
	engine, cookie, st, rt, principal, _ := setupLaneTest(t)
	project := seedProject(t, st, principal)
	seedValidProjectEnvironment(t, st, project, principal.userID)

	scratch := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "Scratch",
		"agent_provider": models.AgentProviderClaudeCode,
	})
	if scratch["lane_kind"] != models.LaneKindScratch {
		t.Fatalf("scratch kind = %#v", scratch["lane_kind"])
	}

	if scratch["project_id"] != nil {
		t.Fatalf("scratch project_id = %#v, want null", scratch["project_id"])
	}

	if rt.execs != 0 {
		t.Fatalf("scratch clone execs = %d, want 0", rt.execs)
	}

	projectLane := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "Project lane",
		"project_id":     project.ID,
		"agent_provider": models.AgentProviderClaudeCode,
	})
	if projectLane["lane_kind"] != models.LaneKindProject {
		t.Fatalf("project kind = %#v", projectLane["lane_kind"])
	}

	if projectLane["project_id"] != project.ID {
		t.Fatalf("project_id = %#v, want %s", projectLane["project_id"], project.ID)
	}

	aliasBody, _ := json.Marshal(map[string]any{
		"name":           "Alias lane",
		"agent_provider": models.AgentProviderClaudeCode,
	})
	aliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/lanes", bytes.NewReader(aliasBody))
	aliasReq.Header.Set("Content-Type", "application/json")
	aliasReq.AddCookie(cookie)

	aliasRec := httptest.NewRecorder()
	engine.ServeHTTP(aliasRec, aliasReq)

	if aliasRec.Code != http.StatusCreated {
		t.Fatalf("alias create = %d body=%s", aliasRec.Code, aliasRec.Body.String())
	}

	var alias map[string]any

	_ = json.Unmarshal(aliasRec.Body.Bytes(), &alias)
	if alias["lane_kind"] != models.LaneKindProject || alias["project_id"] != project.ID {
		t.Fatalf("alias lane = %#v", alias)
	}
}

func TestListLanesGroupedAndDestroy(t *testing.T) {
	engine, cookie, st, rt, principal, _ := setupLaneTest(t)
	project := seedProject(t, st, principal)
	seedValidProjectEnvironment(t, st, project, principal.userID)

	scratch := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "Scratch",
		"agent_provider": models.AgentProviderClaudeCode,
	})
	projectLane := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "With project",
		"project_id":     project.ID,
		"agent_provider": models.AgentProviderClaudeCode,
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/lanes", nil)
	listReq.AddCookie(cookie)

	listRec := httptest.NewRecorder()
	engine.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list = %d body=%s", listRec.Code, listRec.Body.String())
	}

	var listed struct {
		Projects []struct {
			ID    string           `json:"id"`
			Name  string           `json:"name"`
			Lanes []map[string]any `json:"lanes"`
		} `json:"projects"`
		ScratchLanes []map[string]any `json:"scratch_lanes"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}

	if len(listed.ScratchLanes) != 1 || listed.ScratchLanes[0]["id"] != scratch["id"] {
		t.Fatalf("scratch_lanes = %#v", listed.ScratchLanes)
	}

	if len(listed.Projects) != 1 || listed.Projects[0].ID != project.ID {
		t.Fatalf("projects = %#v", listed.Projects)
	}

	if len(listed.Projects[0].Lanes) != 1 || listed.Projects[0].Lanes[0]["id"] != projectLane["id"] {
		t.Fatalf("project lanes = %#v", listed.Projects[0].Lanes)
	}

	if listed.ScratchLanes[0]["has_other_participants"] != false {
		t.Fatalf("has_other_participants = %#v", listed.ScratchLanes[0]["has_other_participants"])
	}

	laneID, _ := scratch["id"].(string)
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/lanes/"+laneID, nil)
	delReq.AddCookie(cookie)

	delRec := httptest.NewRecorder()
	engine.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusNoContent {
		t.Fatalf("destroy = %d body=%s", delRec.Code, delRec.Body.String())
	}

	if len(rt.destroyed) != 1 {
		t.Fatalf("runtime destroyed = %#v", rt.destroyed)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/lanes/"+laneID, nil)
	getReq.AddCookie(cookie)

	getRec := httptest.NewRecorder()
	engine.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get destroyed = %d, want 404", getRec.Code)
	}

	list2Req := httptest.NewRequest(http.MethodGet, "/api/v1/lanes", nil)
	list2Req.AddCookie(cookie)

	list2Rec := httptest.NewRecorder()
	engine.ServeHTTP(list2Rec, list2Req)

	var listed2 struct {
		ScratchLanes []map[string]any `json:"scratch_lanes"`
	}

	_ = json.Unmarshal(list2Rec.Body.Bytes(), &listed2)
	if len(listed2.ScratchLanes) != 0 {
		t.Fatalf("destroyed still listed: %#v", listed2.ScratchLanes)
	}
}

func TestParticipantsHardDeleteAndInviteMultiUse(t *testing.T) {
	engine, cookie, st, _, principal, pool := setupLaneTest(t)

	memberID := insertMemberUser(t, pool, principal.orgID, "member@example.com", "Member", "password1")
	memberCookie := loginCookie(t, engine, "member@example.com", "password1")

	lane := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "Shared",
		"agent_provider": models.AgentProviderClaudeCode,
	})
	laneID, _ := lane["id"].(string)

	addBody, _ := json.Marshal(map[string]string{"user_id": memberID})
	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/lanes/"+laneID+"/participants", bytes.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addReq.AddCookie(cookie)

	addRec := httptest.NewRecorder()
	engine.ServeHTTP(addRec, addReq)

	if addRec.Code != http.StatusCreated {
		t.Fatalf("add participant = %d body=%s", addRec.Code, addRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/lanes", nil)
	listReq.AddCookie(cookie)

	listRec := httptest.NewRecorder()
	engine.ServeHTTP(listRec, listReq)

	var listed struct {
		ScratchLanes []map[string]any `json:"scratch_lanes"`
	}

	_ = json.Unmarshal(listRec.Body.Bytes(), &listed)
	if listed.ScratchLanes[0]["has_other_participants"] != true {
		t.Fatalf("expected has_other_participants true, got %#v", listed.ScratchLanes[0])
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/lanes/"+laneID+"/invites", nil)
	inviteReq.AddCookie(cookie)

	inviteRec := httptest.NewRecorder()
	engine.ServeHTTP(inviteRec, inviteReq)

	if inviteRec.Code != http.StatusOK {
		t.Fatalf("create invite = %d body=%s", inviteRec.Code, inviteRec.Body.String())
	}

	var invite struct {
		Token string `json:"token"`
		ID    string `json:"id"`
	}

	_ = json.Unmarshal(inviteRec.Body.Bytes(), &invite)
	if invite.Token == "" {
		t.Fatal("missing invite token")
	}

	// Idempotent create returns same active invite.
	invite2Req := httptest.NewRequest(http.MethodPost, "/api/v1/lanes/"+laneID+"/invites", nil)
	invite2Req.AddCookie(cookie)

	invite2Rec := httptest.NewRecorder()
	engine.ServeHTTP(invite2Rec, invite2Req)

	var invite2 struct {
		Token string `json:"token"`
	}

	_ = json.Unmarshal(invite2Rec.Body.Bytes(), &invite2)
	if invite2.Token != invite.Token {
		t.Fatalf("idempotent invite token changed: %s vs %s", invite.Token, invite2.Token)
	}

	outsiderID := insertMemberUser(t, pool, principal.orgID, "outsider@example.com", "Outsider", "password1")
	outsiderCookie := loginCookie(t, engine, "outsider@example.com", "password1")

	accept1 := httptest.NewRequest(http.MethodPost, "/api/v1/lane-invites/"+invite.Token+"/accept", nil)
	accept1.AddCookie(outsiderCookie)

	accept1Rec := httptest.NewRecorder()
	engine.ServeHTTP(accept1Rec, accept1)

	if accept1Rec.Code != http.StatusOK {
		t.Fatalf("accept1 = %d body=%s", accept1Rec.Code, accept1Rec.Body.String())
	}

	// Multi-use: same token still works (already participant is ok).
	accept2 := httptest.NewRequest(http.MethodPost, "/api/v1/lane-invites/"+invite.Token+"/accept", nil)
	accept2.AddCookie(outsiderCookie)

	accept2Rec := httptest.NewRecorder()
	engine.ServeHTTP(accept2Rec, accept2)

	if accept2Rec.Code != http.StatusOK {
		t.Fatalf("accept2 = %d body=%s", accept2Rec.Code, accept2Rec.Body.String())
	}

	regenReq := httptest.NewRequest(http.MethodPost, "/api/v1/lanes/"+laneID+"/invites/regenerate", nil)
	regenReq.AddCookie(cookie)

	regenRec := httptest.NewRecorder()
	engine.ServeHTTP(regenRec, regenReq)

	if regenRec.Code != http.StatusOK {
		t.Fatalf("regenerate = %d body=%s", regenRec.Code, regenRec.Body.String())
	}

	var regen struct {
		Token string `json:"token"`
	}

	_ = json.Unmarshal(regenRec.Body.Bytes(), &regen)
	if regen.Token == "" || regen.Token == invite.Token {
		t.Fatalf("regen token = %q old = %q", regen.Token, invite.Token)
	}

	_ = insertMemberUser(t, pool, principal.orgID, "another@example.com", "Another", "password1")
	anotherCookie := loginCookie(t, engine, "another@example.com", "password1")
	oldAccept := httptest.NewRequest(http.MethodPost, "/api/v1/lane-invites/"+invite.Token+"/accept", nil)
	oldAccept.AddCookie(anotherCookie)

	oldAcceptRec := httptest.NewRecorder()
	engine.ServeHTTP(oldAcceptRec, oldAccept)

	if oldAcceptRec.Code != http.StatusConflict {
		t.Fatalf("old token accept = %d, want 409 body=%s", oldAcceptRec.Code, oldAcceptRec.Body.String())
	}

	leaveReq := httptest.NewRequest(http.MethodPost, "/api/v1/lanes/"+laneID+"/leave", nil)
	leaveReq.AddCookie(memberCookie)

	leaveRec := httptest.NewRecorder()
	engine.ServeHTTP(leaveRec, leaveReq)

	if leaveRec.Code != http.StatusNoContent {
		t.Fatalf("leave = %d body=%s", leaveRec.Code, leaveRec.Body.String())
	}

	parts, err := st.ListLaneParticipants(context.Background(), laneID)
	if err != nil {
		t.Fatalf("list participants: %v", err)
	}

	for _, p := range parts {
		if p.UserID == memberID {
			t.Fatal("member seat still present after leave")
		}
	}

	removeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/lanes/"+laneID+"/participants/"+outsiderID, nil)
	removeReq.AddCookie(cookie)

	removeRec := httptest.NewRecorder()
	engine.ServeHTTP(removeRec, removeReq)

	if removeRec.Code != http.StatusNoContent {
		t.Fatalf("remove = %d body=%s", removeRec.Code, removeRec.Body.String())
	}

	ownerLeave := httptest.NewRequest(http.MethodPost, "/api/v1/lanes/"+laneID+"/leave", nil)
	ownerLeave.AddCookie(cookie)

	ownerLeaveRec := httptest.NewRecorder()
	engine.ServeHTTP(ownerLeaveRec, ownerLeave)

	if ownerLeaveRec.Code != http.StatusConflict {
		t.Fatalf("owner leave = %d, want 409", ownerLeaveRec.Code)
	}

	previewReq := httptest.NewRequest(http.MethodGet, "/api/v1/lane-invites/"+regen.Token, nil)
	previewRec := httptest.NewRecorder()
	engine.ServeHTTP(previewRec, previewReq)

	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview = %d body=%s", previewRec.Code, previewRec.Body.String())
	}

	var preview map[string]any

	_ = json.Unmarshal(previewRec.Body.Bytes(), &preview)
	if preview["invited_by_display_name"] != "Owner" {
		t.Fatalf("invited_by_display_name = %#v", preview["invited_by_display_name"])
	}

	if preview["pending"] != true {
		t.Fatalf("pending = %#v", preview["pending"])
	}
}

func TestPatchLaneModelAndAgentSwitchDefaults(t *testing.T) {
	engine, cookie, st, _, principal, _ := setupLaneTest(t)
	if err := st.SetOrganizationSecret(
		context.Background(),
		principal.orgID,
		models.SecretNameOpenAIAPIKey,
		"sk-openai",
		testConfig().SessionSecret,
		principal.userID,
	); err != nil {
		t.Fatalf("SetOrganizationSecret openai: %v", err)
	}

	lane := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "Settings",
		"agent_provider": models.AgentProviderClaudeCode,
	})
	if lane["model_source"] != "anthropic" {
		t.Fatalf("create model_source = %#v", lane["model_source"])
	}

	if lane["agent_model"] != "claude-sonnet-4-5" {
		t.Fatalf("create agent_model = %#v", lane["agent_model"])
	}

	if lane["reasoning_effort"] != "medium" {
		t.Fatalf("create reasoning_effort = %#v", lane["reasoning_effort"])
	}

	laneID, _ := lane["id"].(string)

	modelsReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/lane-agents/"+models.AgentProviderClaudeCode+"/models?source=anthropic",
		nil,
	)
	modelsReq.AddCookie(cookie)

	modelsRec := httptest.NewRecorder()
	engine.ServeHTTP(modelsRec, modelsReq)

	if modelsRec.Code != http.StatusOK {
		t.Fatalf("list models = %d body=%s", modelsRec.Code, modelsRec.Body.String())
	}

	var modelList struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(modelsRec.Body.Bytes(), &modelList); err != nil {
		t.Fatalf("decode models: %v", err)
	}

	if len(modelList.Models) < 1 {
		t.Fatal("expected static claude models")
	}

	// Reject unknown model.
	badBody, err := json.Marshal(map[string]any{"agent_model": "not-real"})
	if err != nil {
		t.Fatalf("marshal bad model: %v", err)
	}

	badReq := httptest.NewRequest(http.MethodPatch, "/api/v1/lanes/"+laneID, bytes.NewReader(badBody))
	badReq.Header.Set("Content-Type", "application/json")
	badReq.AddCookie(cookie)

	badRec := httptest.NewRecorder()
	engine.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bad model = %d body=%s", badRec.Code, badRec.Body.String())
	}

	// Reject invalid reasoning effort.
	badEffortBody, err := json.Marshal(map[string]any{
		"agent_model":      "claude-sonnet-4-5",
		"reasoning_effort": "not-an-effort",
	})
	if err != nil {
		t.Fatalf("marshal bad effort: %v", err)
	}

	badEffortReq := httptest.NewRequest(http.MethodPatch, "/api/v1/lanes/"+laneID, bytes.NewReader(badEffortBody))
	badEffortReq.Header.Set("Content-Type", "application/json")
	badEffortReq.AddCookie(cookie)

	badEffortRec := httptest.NewRecorder()
	engine.ServeHTTP(badEffortRec, badEffortReq)

	if badEffortRec.Code != http.StatusBadRequest {
		t.Fatalf("bad effort = %d body=%s", badEffortRec.Code, badEffortRec.Body.String())
	}

	// Patch model + effort.
	patchBody, err := json.Marshal(map[string]any{
		"agent_model":      "claude-opus-4-5",
		"reasoning_effort": "high",
	})
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/lanes/"+laneID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.AddCookie(cookie)

	patchRec := httptest.NewRecorder()
	engine.ServeHTTP(patchRec, patchReq)

	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch model = %d body=%s", patchRec.Code, patchRec.Body.String())
	}

	var patched map[string]any
	if err := json.Unmarshal(patchRec.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patched: %v", err)
	}

	if patched["agent_model"] != "claude-opus-4-5" || patched["reasoning_effort"] != "high" {
		t.Fatalf("patched = %#v", patched)
	}

	// Switch agent → invalid prior model resets to Codex default.
	switchBody, err := json.Marshal(map[string]any{
		"agent_provider": models.AgentProviderCodex,
	})
	if err != nil {
		t.Fatalf("marshal switch: %v", err)
	}

	switchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/lanes/"+laneID, bytes.NewReader(switchBody))
	switchReq.Header.Set("Content-Type", "application/json")
	switchReq.AddCookie(cookie)

	switchRec := httptest.NewRecorder()
	engine.ServeHTTP(switchRec, switchReq)

	if switchRec.Code != http.StatusOK {
		t.Fatalf("switch agent = %d body=%s", switchRec.Code, switchRec.Body.String())
	}

	var switched map[string]any
	if err := json.Unmarshal(switchRec.Body.Bytes(), &switched); err != nil {
		t.Fatalf("decode switched: %v", err)
	}

	if switched["agent_provider"] != models.AgentProviderCodex {
		t.Fatalf("provider = %#v", switched["agent_provider"])
	}

	if switched["model_source"] != "openai" {
		t.Fatalf("switched model_source = %#v", switched["model_source"])
	}

	if switched["agent_model"] != "gpt-5.1-codex" {
		t.Fatalf("switched model = %#v, want gpt-5.1-codex", switched["agent_model"])
	}

	if switched["reasoning_effort"] != "medium" {
		t.Fatalf("switched effort = %#v", switched["reasoning_effort"])
	}
}

func TestStoreLaneKindConstraints(t *testing.T) {
	_, _, st, _, principal, _ := setupLaneTest(t)
	img := "farplane/test:latest"

	medium := "medium"

	_, err := st.CreateLane(context.Background(), store.CreateLaneInput{
		OrganizationID:     principal.orgID,
		OwnerUserID:        principal.userID,
		Name:               "bad project",
		LaneKind:           models.LaneKindProject,
		DockerfileSnapshot: "FROM scratch",
		ImageReference:     &img,
		AgentProvider:      models.AgentProviderClaudeCode,
		ModelSource:        "anthropic",
		AgentModel:         "claude-sonnet-4-5",
		ReasoningEffort:    &medium,
	})
	if err == nil {
		t.Fatal("expected error for project lane without project_id")
	}

	lane, err := st.CreateLane(context.Background(), store.CreateLaneInput{
		OrganizationID:     principal.orgID,
		OwnerUserID:        principal.userID,
		Name:               "ok scratch",
		LaneKind:           models.LaneKindScratch,
		DockerfileSnapshot: "FROM scratch",
		ImageReference:     &img,
		AgentProvider:      models.AgentProviderClaudeCode,
		ModelSource:        "anthropic",
		AgentModel:         "claude-sonnet-4-5",
		ReasoningEffort:    &medium,
	})
	if err != nil {
		t.Fatalf("CreateLane scratch: %v", err)
	}

	if lane.LaneKind != models.LaneKindScratch || lane.ProjectID != nil {
		t.Fatalf("lane = %#v", lane)
	}
}
