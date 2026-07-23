package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/envgen"
	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type stubGenerator struct {
	dockerfile string
	log        string
	err        error
}

func (s stubGenerator) Generate(ctx context.Context, req envgen.Request) (envgen.Result, error) {
	_ = ctx
	_ = req
	if s.err != nil {
		return envgen.Result{}, s.err
	}
	return envgen.Result{DockerfileText: s.dockerfile, Log: s.log}, nil
}

func TestScratchEnvironmentGetUpsertValidate(t *testing.T) {
	engine, cookie, _, _, _, _ := setupLaneTest(t)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/scratch-environment", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	engine.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get scratch = %d body=%s", getRec.Code, getRec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(getRec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env["validation_status"] != models.EnvironmentValidationValid {
		t.Fatalf("setup should leave scratch valid, got %#v", env["validation_status"])
	}

	body, _ := json.Marshal(map[string]string{
		"dockerfile_text": "FROM debian:bookworm-slim\nRUN echo hi\n",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/scratch-environment", bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.AddCookie(cookie)
	putRec := httptest.NewRecorder()
	engine.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("put scratch = %d body=%s", putRec.Code, putRec.Body.String())
	}
	_ = json.Unmarshal(putRec.Body.Bytes(), &env)
	if env["validation_status"] != models.EnvironmentValidationInvalid {
		t.Fatalf("after edit want invalid, got %#v", env["validation_status"])
	}

	valReq := httptest.NewRequest(http.MethodPost, "/api/v1/scratch-environment/validate", nil)
	valReq.AddCookie(cookie)
	valRec := httptest.NewRecorder()
	engine.ServeHTTP(valRec, valReq)
	if valRec.Code != http.StatusOK {
		t.Fatalf("validate scratch = %d body=%s", valRec.Code, valRec.Body.String())
	}
	_ = json.Unmarshal(valRec.Body.Bytes(), &env)
	if env["validation_status"] != models.EnvironmentValidationValid {
		t.Fatalf("after validate want valid, got %#v", env["validation_status"])
	}
}

func TestProjectEnvironmentGenerateAndLaneCreate(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	st := store.New(pool)
	rt := &fakeRuntime{}
	base := "FROM debian:bookworm-slim\nWORKDIR /workspace\nCOPY bridge /opt/farplane/bridge\nEXPOSE 7420\nENTRYPOINT [\"node\", \"/opt/farplane/bridge/bridge.js\"]\n"
	engine := httpapi.New(
		pool,
		testConfig(),
		httpapi.WithRuntime(rt),
		httpapi.WithEnvironmentGenerator(stubGenerator{
			dockerfile: base,
			log:        "stub discovery",
		}),
		httpapi.WithProjectWorkspaceClone(func(ctx context.Context, project models.Project) (string, func(), error) {
			_ = ctx
			_ = project
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
				return "", nil, err
			}
			return dir, func() {}, nil
		}),
	)

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
	if err := st.SetOrganizationSecret(
		context.Background(),
		principal.orgID,
		models.SecretNameAnthropicAPIKey,
		"sk-test",
		testConfig().SessionSecret,
		principal.userID,
	); err != nil {
		t.Fatalf("secret: %v", err)
	}
	if _, err := st.EnsureScratchEnvironment(context.Background(), principal.orgID, principal.userID); err != nil {
		t.Fatalf("scratch: %v", err)
	}
	if _, err := st.CompleteScratchEnvironmentValidation(
		context.Background(), principal.orgID, true, "farplane/test:latest", "ok",
	); err != nil {
		t.Fatalf("validate scratch: %v", err)
	}
	project := seedProject(t, st, principal)

	missing := createLaneExpectError(t, engine, cookie, map[string]any{
		"name":           "Too soon",
		"project_id":     project.ID,
		"agent_provider": models.AgentProviderClaudeCode,
	}, http.StatusBadRequest)
	if !strings.Contains(missing, "Project Environment") {
		t.Fatalf("error = %q", missing)
	}

	genReq := httptest.NewRequest(
		http.MethodPost, "/api/v1/projects/"+project.ID+"/environment/generate", bytes.NewReader([]byte("{}")),
	)
	genReq.Header.Set("Content-Type", "application/json")
	genReq.AddCookie(cookie)
	genRec := httptest.NewRecorder()
	engine.ServeHTTP(genRec, genReq)
	if genRec.Code != http.StatusOK {
		t.Fatalf("generate = %d body=%s", genRec.Code, genRec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(genRec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode env: %v", err)
	}
	if env["generation_status"] != models.EnvironmentGenerationIdle {
		t.Fatalf("generation_status = %#v", env["generation_status"])
	}
	if !strings.Contains(env["dockerfile_text"].(string), "FROM debian:bookworm-slim") {
		t.Fatalf("dockerfile = %#v", env["dockerfile_text"])
	}

	valReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/environment/validate", nil)
	valReq.AddCookie(cookie)
	valRec := httptest.NewRecorder()
	engine.ServeHTTP(valRec, valReq)
	if valRec.Code != http.StatusOK {
		t.Fatalf("validate project env = %d body=%s", valRec.Code, valRec.Body.String())
	}

	lane := createLaneHTTP(t, engine, cookie, map[string]any{
		"name":           "Ready",
		"project_id":     project.ID,
		"agent_provider": models.AgentProviderClaudeCode,
	})
	if lane["lane_kind"] != models.LaneKindProject {
		t.Fatalf("lane = %#v", lane)
	}
}

func createLaneExpectError(
	t *testing.T,
	engine http.Handler,
	cookie *http.Cookie,
	body map[string]any,
	wantStatus int,
) string {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lanes", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("create lane status = %d body=%s, want %d", rec.Code, rec.Body.String(), wantStatus)
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	msg, _ := out["error"].(string)
	return msg
}
