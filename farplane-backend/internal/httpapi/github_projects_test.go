package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/auth"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
	"github.com/farplane/farplane/farplane-backend/internal/models"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

type fakeGitHub struct {
	installation githubapp.Installation
	repos        []githubapp.Repository
	webhookOK    bool
	tokenCalls   int
}

func (f *fakeGitHub) InstallURL(state string) string {
	return "https://github.com/apps/farplane/installations/new?state=" + state
}

func (f *fakeGitHub) GetInstallation(ctx context.Context, installationID int64) (githubapp.Installation, error) {
	_ = ctx
	out := f.installation
	out.ID = installationID
	return out, nil
}

func (f *fakeGitHub) CreateInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	_ = ctx
	_ = installationID
	f.tokenCalls++
	return "ghs_fake", time.Now().UTC().Add(time.Hour), nil
}

func (f *fakeGitHub) ListInstallationRepositories(ctx context.Context, installationToken string) ([]githubapp.Repository, error) {
	_ = ctx
	_ = installationToken
	return f.repos, nil
}

func (f *fakeGitHub) VerifyWebhookSignature(body []byte, signatureHeader string) bool {
	_ = body
	_ = signatureHeader
	return f.webhookOK
}

func setupAuthedEngine(
	t *testing.T,
	gh httpapi.GitHubApp,
) (http.Handler, *http.Cookie, *store.Store, *pgxpool.Pool) {
	t.Helper()
	pool := openMigratedTestDB(t, true)
	st := store.New(pool)
	engine := httpapi.New(pool, testConfig(), httpapi.WithGitHubApp(gh))

	rec := postSetup(engine, map[string]string{
		"organization_name": "Acme",
		"email":             "owner@example.com",
		"display_name":      "Owner",
		"password":          "password1",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d body=%s", rec.Code, rec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "farplane_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("missing session cookie")
	}
	return engine, sessionCookie, st, pool
}

func TestGitHubInstallStartCallbackAndRepoUnion(t *testing.T) {
	fake := &fakeGitHub{
		installation: githubapp.Installation{
			RepositorySelection: "selected",
			Account: struct {
				ID    int64  `json:"id"`
				Login string `json:"login"`
				Type  string `json:"type"`
			}{ID: 11, Login: "alice", Type: "User"},
		},
		repos: []githubapp.Repository{
			{
				ID: 1001, FullName: "alice/private-api", DefaultBranch: "main",
				Private: true, HTMLURL: "https://github.com/alice/private-api",
			},
		},
	}
	engine, cookie, st, _ := setupAuthedEngine(t, fake)

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/github/install/start", nil)
	startReq.AddCookie(cookie)
	startRec := httptest.NewRecorder()
	engine.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("install start = %d body=%s", startRec.Code, startRec.Body.String())
	}
	var startBody struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(startRec.Body.Bytes(), &startBody); err != nil {
		t.Fatalf("decode start: %v", err)
	}
	_, state, ok := strings.Cut(startBody.URL, "state=")
	if !ok || state == "" {
		t.Fatalf("install url missing state: %q", startBody.URL)
	}
	parsed, err := auth.ParseGitHubInstallState(testConfig().SessionSecret, state, time.Now().UTC())
	if err != nil {
		t.Fatalf("ParseGitHubInstallState: %v", err)
	}

	cbReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/callback?installation_id=4242&setup_action=install&state="+state,
		nil,
	)
	cbRec := httptest.NewRecorder()
	engine.ServeHTTP(cbRec, cbReq)
	if cbRec.Code != http.StatusFound {
		t.Fatalf("callback = %d body=%s", cbRec.Code, cbRec.Body.String())
	}
	if loc := cbRec.Header().Get("Location"); loc != "http://localhost:3000/settings/github?github=connected" {
		t.Fatalf("Location = %q", loc)
	}
	if fake.tokenCalls < 1 {
		t.Fatal("expected repo sync via installation token")
	}

	orgInst, err := st.UpsertGitHubInstallation(context.Background(), store.UpsertGitHubInstallationInput{
		OrganizationID:       parsed.OrganizationID,
		GitHubInstallationID: 9001,
		GitHubAccountID:      22,
		GitHubAccountLogin:   "acme",
		GitHubAccountType:    models.GitHubAccountTypeOrganization,
		RepositorySelection:  models.GitHubRepositorySelectionAll,
		ConnectedByUserID:    parsed.UserID,
	})
	if err != nil {
		t.Fatalf("UpsertGitHubInstallation org: %v", err)
	}
	if err := st.ReplaceGitHubRepositories(context.Background(), orgInst.ID, []store.GitHubRepoSync{
		{
			GitHubRepositoryID: 1001, FullName: "acme/private-api", DefaultBranch: "main",
			Private: true, HTMLURL: "https://github.com/acme/private-api",
		},
		{
			GitHubRepositoryID: 2002, FullName: "acme/web", DefaultBranch: "main",
			Private: false, HTMLURL: "https://github.com/acme/web",
		},
	}); err != nil {
		t.Fatalf("ReplaceGitHubRepositories: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/github/repositories", nil)
	listReq.AddCookie(cookie)
	listRec := httptest.NewRecorder()
	engine.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list repos = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Repositories []struct {
			GitHubRepositoryID int64  `json:"github_repository_id"`
			FullName           string `json:"full_name"`
			AccountType        string `json:"github_account_type"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode repos: %v", err)
	}
	if len(listBody.Repositories) != 2 {
		t.Fatalf("repos len = %d, want 2: %+v", len(listBody.Repositories), listBody.Repositories)
	}
	byID := map[int64]string{}
	for _, repo := range listBody.Repositories {
		byID[repo.GitHubRepositoryID] = repo.FullName + "|" + repo.AccountType
	}
	if byID[1001] != "acme/private-api|Organization" {
		t.Fatalf("repo 1001 = %q, want org preference", byID[1001])
	}
	if byID[2002] != "acme/web|Organization" {
		t.Fatalf("repo 2002 = %q", byID[2002])
	}
}

func TestDisconnectACLAndProjectCreate(t *testing.T) {
	fake := &fakeGitHub{
		installation: githubapp.Installation{
			RepositorySelection: "all",
			Account: struct {
				ID    int64  `json:"id"`
				Login string `json:"login"`
				Type  string `json:"type"`
			}{ID: 1, Login: "alice", Type: "User"},
		},
		repos: []githubapp.Repository{
			{
				ID: 55, FullName: "alice/app", DefaultBranch: "main",
				Private: true, HTMLURL: "https://github.com/alice/app",
			},
		},
	}
	engine, ownerCookie, st, pool := setupAuthedEngine(t, fake)

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(ownerCookie)
	meRec := httptest.NewRecorder()
	engine.ServeHTTP(meRec, meReq)
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}

	memberID := insertMemberUser(t, pool, me.Organization.ID, "member@example.com", "Member", "password1")
	_ = memberID
	memberCookie := loginCookie(t, engine, "member@example.com", "password1")

	inst, err := st.UpsertGitHubInstallation(context.Background(), store.UpsertGitHubInstallationInput{
		OrganizationID:       me.Organization.ID,
		GitHubInstallationID: 77,
		GitHubAccountID:      1,
		GitHubAccountLogin:   "alice",
		GitHubAccountType:    models.GitHubAccountTypeUser,
		RepositorySelection:  models.GitHubRepositorySelectionAll,
		ConnectedByUserID:    me.User.ID,
	})
	if err != nil {
		t.Fatalf("upsert install: %v", err)
	}
	if err := st.ReplaceGitHubRepositories(context.Background(), inst.ID, []store.GitHubRepoSync{
		{
			GitHubRepositoryID: 55, FullName: "alice/app", DefaultBranch: "main",
			Private: true, HTMLURL: "https://github.com/alice/app",
		},
	}); err != nil {
		t.Fatalf("sync repos: %v", err)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/github/installations/"+inst.ID, nil)
	delReq.AddCookie(memberCookie)
	delRec := httptest.NewRecorder()
	engine.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusForbidden {
		t.Fatalf("member disconnect = %d, want 403", delRec.Code)
	}

	createBody, _ := json.Marshal(map[string]any{
		"name":                 "App",
		"github_repository_id": 55,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(memberCookie)
	createRec := httptest.NewRecorder()
	engine.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create project = %d body=%s", createRec.Code, createRec.Body.String())
	}

	badBody, _ := json.Marshal(map[string]any{
		"name":                 "Missing",
		"github_repository_id": 99999,
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(badBody))
	badReq.Header.Set("Content-Type", "application/json")
	badReq.AddCookie(memberCookie)
	badRec := httptest.NewRecorder()
	engine.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("missing repo create = %d, want 400", badRec.Code)
	}

	if err := st.SoftRemoveGitHubRepositories(context.Background(), inst.ID, []int64{55}); err != nil {
		t.Fatalf("soft remove: %v", err)
	}
	project, err := st.GetProject(context.Background(), mustProjectID(t, createRec.Body.Bytes()))
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if project.GitHubAccessStatus != models.ProjectGitHubAccessRevoked {
		t.Fatalf("access = %q, want revoked", project.GitHubAccessStatus)
	}

	againBody, _ := json.Marshal(map[string]any{
		"name":                 "App2",
		"github_repository_id": 55,
	})
	againReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(againBody))
	againReq.Header.Set("Content-Type", "application/json")
	againReq.AddCookie(ownerCookie)
	againRec := httptest.NewRecorder()
	engine.ServeHTTP(againRec, againReq)
	if againRec.Code != http.StatusBadRequest {
		t.Fatalf("revoked repo create = %d, want 400 body=%s", againRec.Code, againRec.Body.String())
	}

	delOwner := httptest.NewRequest(http.MethodDelete, "/api/v1/github/installations/"+inst.ID, nil)
	delOwner.AddCookie(ownerCookie)
	delOwnerRec := httptest.NewRecorder()
	engine.ServeHTTP(delOwnerRec, delOwner)
	if delOwnerRec.Code != http.StatusNoContent {
		t.Fatalf("owner disconnect = %d", delOwnerRec.Code)
	}
}

func TestGitHubInstallPreservesConnectedByAndRejectsCrossOrg(t *testing.T) {
	fake := &fakeGitHub{
		installation: githubapp.Installation{
			RepositorySelection: "all",
			Account: struct {
				ID    int64  `json:"id"`
				Login string `json:"login"`
				Type  string `json:"type"`
			}{ID: 1, Login: "alice", Type: "User"},
		},
		repos: []githubapp.Repository{
			{
				ID: 55, FullName: "alice/app", DefaultBranch: "main",
				Private: true, HTMLURL: "https://github.com/alice/app",
			},
		},
	}
	engine, ownerCookie, st, pool := setupAuthedEngine(t, fake)
	principal := mustPrincipal(t, engine)
	memberID := insertMemberUser(t, pool, principal.orgID, "member@example.com", "Member", "password1")
	memberCookie := loginCookie(t, engine, "member@example.com", "password1")

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/github/install/start", nil)
	startReq.AddCookie(ownerCookie)
	startRec := httptest.NewRecorder()
	engine.ServeHTTP(startRec, startReq)
	var startBody struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(startRec.Body.Bytes(), &startBody)
	_, ownerState, _ := strings.Cut(startBody.URL, "state=")

	cbReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/callback?installation_id=4242&setup_action=install&state="+ownerState,
		nil,
	)
	cbRec := httptest.NewRecorder()
	engine.ServeHTTP(cbRec, cbReq)
	if cbRec.Code != http.StatusFound {
		t.Fatalf("owner callback = %d body=%s", cbRec.Code, cbRec.Body.String())
	}
	if loc := cbRec.Header().Get("Location"); !strings.Contains(loc, "github=connected") {
		t.Fatalf("Location = %q", loc)
	}

	inst, err := st.GetGitHubInstallationByGitHubID(context.Background(), 4242)
	if err != nil {
		t.Fatalf("get install: %v", err)
	}
	if inst.ConnectedByUserID != principal.userID {
		t.Fatalf("connected_by = %q, want owner %q", inst.ConnectedByUserID, principal.userID)
	}

	// Member re-runs install callback for the same installation — must not steal connected_by.
	memberStart := httptest.NewRequest(http.MethodPost, "/api/v1/github/install/start", nil)
	memberStart.AddCookie(memberCookie)
	memberStartRec := httptest.NewRecorder()
	engine.ServeHTTP(memberStartRec, memberStart)
	var memberStartBody struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(memberStartRec.Body.Bytes(), &memberStartBody)
	_, memberState, _ := strings.Cut(memberStartBody.URL, "state=")

	memberCB := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/callback?installation_id=4242&setup_action=install&state="+memberState,
		nil,
	)
	memberCBRec := httptest.NewRecorder()
	engine.ServeHTTP(memberCBRec, memberCB)
	if memberCBRec.Code != http.StatusFound {
		t.Fatalf("member callback = %d", memberCBRec.Code)
	}
	if loc := memberCBRec.Header().Get("Location"); !strings.Contains(loc, "github=connected") {
		t.Fatalf("member Location = %q", loc)
	}
	inst2, err := st.GetGitHubInstallationByGitHubID(context.Background(), 4242)
	if err != nil {
		t.Fatalf("get install after member: %v", err)
	}
	if inst2.ConnectedByUserID != principal.userID {
		t.Fatalf("connected_by stolen: got %q (member=%q)", inst2.ConnectedByUserID, memberID)
	}

	// A second Farplane organization cannot claim an active installation.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO organizations (id, name, created_at, updated_at)
		VALUES ('00000000-0000-4000-8000-000000000099', 'Other', NOW(), NOW())
	`)
	if err != nil {
		t.Fatalf("insert other org: %v", err)
	}
	_, err = st.UpsertGitHubInstallation(context.Background(), store.UpsertGitHubInstallationInput{
		OrganizationID:       "00000000-0000-4000-8000-000000000099",
		GitHubInstallationID: 4242,
		GitHubAccountID:      1,
		GitHubAccountLogin:   "alice",
		GitHubAccountType:    models.GitHubAccountTypeUser,
		RepositorySelection:  models.GitHubRepositorySelectionAll,
		ConnectedByUserID:    principal.userID,
	})
	if !errors.Is(err, store.ErrGitHubInstallationOwned) {
		t.Fatalf("cross-org upsert err = %v, want ErrGitHubInstallationOwned", err)
	}
}

func TestGitHubWebhookStoreFailureReturns500(t *testing.T) {
	failing := &failingListGitHub{fakeGitHub: &fakeGitHub{webhookOK: true}}
	engine, _, st, _ := setupAuthedEngine(t, failing)
	principal := mustPrincipal(t, engine)

	_, err := st.UpsertGitHubInstallation(context.Background(), store.UpsertGitHubInstallationInput{
		OrganizationID:       principal.orgID,
		GitHubInstallationID: 777,
		GitHubAccountID:      1,
		GitHubAccountLogin:   "alice",
		GitHubAccountType:    models.GitHubAccountTypeUser,
		RepositorySelection:  models.GitHubRepositorySelectionAll,
		ConnectedByUserID:    principal.userID,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Unsuspend re-syncs repositories; a GitHub list failure must surface as 5xx
	// so GitHub retries instead of silently dropping the event.
	body := []byte(`{"action":"unsuspend","installation":{"id":777}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-Hub-Signature-256", "sha256=00")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("webhook = %d, want 500 body=%s", rec.Code, rec.Body.String())
	}
}

type failingListGitHub struct {
	*fakeGitHub
}

func (f *failingListGitHub) ListInstallationRepositories(ctx context.Context, installationToken string) ([]githubapp.Repository, error) {
	_ = ctx
	_ = installationToken
	return nil, errors.New("github list failed")
}

func TestGitHubWebhookSignatureAndUninstall(t *testing.T) {
	fake := &fakeGitHub{webhookOK: true}
	engine, _, st, _ := setupAuthedEngine(t, fake)

	principal := mustPrincipal(t, engine)
	inst, err := st.UpsertGitHubInstallation(context.Background(), store.UpsertGitHubInstallationInput{
		OrganizationID:       principal.orgID,
		GitHubInstallationID: 321,
		GitHubAccountID:      9,
		GitHubAccountLogin:   "alice",
		GitHubAccountType:    models.GitHubAccountTypeUser,
		RepositorySelection:  models.GitHubRepositorySelectionAll,
		ConnectedByUserID:    principal.userID,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_ = inst

	body := []byte(`{"action":"deleted","installation":{"id":321}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "installation")
	req.Header.Set("X-Hub-Signature-256", "sha256=00")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("webhook = %d body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetGitHubInstallationByGitHubID(context.Background(), 321)
	if err != nil {
		t.Fatalf("get install: %v", err)
	}
	if got.UninstalledAt == nil {
		t.Fatal("expected uninstalled_at set")
	}

	fake.webhookOK = false
	bad := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhook", bytes.NewReader(body))
	bad.Header.Set("X-GitHub-Event", "installation")
	bad.Header.Set("X-Hub-Signature-256", "sha256=00")
	badRec := httptest.NewRecorder()
	engine.ServeHTTP(badRec, bad)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad sig = %d, want 401", badRec.Code)
	}
}

type testPrincipal struct {
	userID string
	orgID  string
}

func mustPrincipal(t *testing.T, engine http.Handler) testPrincipal {
	t.Helper()
	// Re-login as owner from known setup credentials.
	cookie := loginCookie(t, engine, "owner@example.com", "password1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d body=%s", rec.Code, rec.Body.String())
	}
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	return testPrincipal{userID: me.User.ID, orgID: me.Organization.ID}
}

func loginCookie(t *testing.T, engine http.Handler, email, password string) *http.Cookie {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "farplane_session" {
			return c
		}
	}
	t.Fatal("missing session cookie after login")
	return nil
}

func mustProjectID(t *testing.T, raw []byte) string {
	t.Helper()
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.ID == "" {
		t.Fatalf("project id missing: %s", raw)
	}
	return body.ID
}

func insertMemberUser(t *testing.T, pool *pgxpool.Pool, organizationID, email, displayName, password string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	now := time.Now().UTC()
	var userID string
	err = pool.QueryRow(context.Background(), `
		INSERT INTO users (email, password_hash, display_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id
	`, email, hash, displayName, now).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO organization_members (organization_id, user_id, role, created_at)
		VALUES ($1, $2, $3, $4)
	`, organizationID, userID, models.OrganizationRoleMember, now)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	return userID
}
