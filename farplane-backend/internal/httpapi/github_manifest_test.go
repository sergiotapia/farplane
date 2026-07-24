package httpapi_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
	"github.com/farplane/farplane/farplane-backend/internal/store"
)

func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	der := x509.MarshalPKCS1PrivateKey(key)

	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

func TestGitHubManifestStartAndCallback(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	pemKey := testRSAPrivateKeyPEM(t)

	cfg := testConfig()
	cfg.AppAPIBaseURL = "https://farplane-test.example.com"
	engine := httpapi.New(
		pool,
		cfg,
		httpapi.WithManifestConvert(func(ctx context.Context, code string) (githubapp.ManifestApp, error) {
			_ = ctx

			if code != "manifest-code" {
				t.Fatalf("code = %q", code)
			}

			return githubapp.ManifestApp{
				ID:            555,
				Slug:          "farplane",
				Name:          "Farplane",
				ClientID:      "cid",
				ClientSecret:  "csec",
				WebhookSecret: "whsec",
				PEM:           pemKey,
			}, nil
		}),
	)

	setupRec := postSetup(engine, map[string]string{
		"organization_name": "Acme",
		"email":             "owner@example.com",
		"display_name":      "Owner",
		"password":          "password1",
	})
	if setupRec.Code != http.StatusCreated {
		t.Fatalf("setup = %d body=%s", setupRec.Code, setupRec.Body.String())
	}

	var cookie *http.Cookie

	for _, c := range setupRec.Result().Cookies() {
		if c.Name == "farplane_session" {
			cookie = c
			break
		}
	}

	if cookie == nil {
		t.Fatal("missing session")
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/github/app/manifest/start", nil)
	startReq.AddCookie(cookie)

	startRec := httptest.NewRecorder()
	engine.ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("manifest start = %d body=%s", startRec.Code, startRec.Body.String())
	}

	var startBody struct {
		Action   string `json:"action"`
		Manifest string `json:"manifest"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal(startRec.Body.Bytes(), &startBody); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if startBody.Action != "https://github.com/settings/apps/new" {
		t.Fatalf("action = %q", startBody.Action)
	}

	if startBody.State == "" || startBody.Manifest == "" {
		t.Fatal("expected state and manifest")
	}

	var manifest map[string]any
	if err := json.Unmarshal([]byte(startBody.Manifest), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}

	if manifest["name"] != "Farplane AI (Acme)" {
		t.Fatalf("manifest name = %#v", manifest["name"])
	}

	cbReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/app/manifest/callback?code=manifest-code&state="+startBody.State,
		nil,
	)
	cbRec := httptest.NewRecorder()
	engine.ServeHTTP(cbRec, cbReq)

	if cbRec.Code != http.StatusFound {
		t.Fatalf("manifest callback = %d body=%s", cbRec.Code, cbRec.Body.String())
	}

	if loc := cbRec.Header().Get("Location"); loc != "http://localhost:3000/settings/github?github=app_created" {
		t.Fatalf("Location = %q", loc)
	}

	st := store.New(pool)

	creds, err := st.GetGitHubAppCredentials(context.Background(), testConfig().SessionSecret)
	if err != nil {
		t.Fatalf("GetGitHubAppCredentials: %v", err)
	}

	if creds.GitHubAppID != 555 || creds.GitHubAppSlug != "farplane" || creds.WebhookSecret != "whsec" {
		t.Fatalf("creds = %+v", creds)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/github/installations", nil)
	listReq.AddCookie(cookie)

	listRec := httptest.NewRecorder()
	engine.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("installations = %d", listRec.Code)
	}

	var listBody struct {
		Configured bool `json:"configured"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}

	if !listBody.Configured {
		t.Fatal("expected configured true after manifest")
	}

	// Second manifest start must fail once the App is loaded in-process.
	secondStart := httptest.NewRequest(http.MethodPost, "/api/v1/github/app/manifest/start", nil)
	secondStart.AddCookie(cookie)

	secondStartRec := httptest.NewRecorder()
	engine.ServeHTTP(secondStartRec, secondStart)

	if secondStartRec.Code != http.StatusConflict {
		t.Fatalf("second manifest start = %d, want 409", secondStartRec.Code)
	}

	// Replaying the callback must not overwrite credentials.
	cb2 := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/app/manifest/callback?code=manifest-code&state="+startBody.State,
		nil,
	)
	cb2Rec := httptest.NewRecorder()
	engine.ServeHTTP(cb2Rec, cb2)

	if cb2Rec.Code != http.StatusFound {
		t.Fatalf("second callback = %d", cb2Rec.Code)
	}

	if loc := cb2Rec.Header().Get("Location"); !strings.Contains(loc, "github_app_already_configured") {
		t.Fatalf("second callback Location = %q, want already configured", loc)
	}

	creds2, err := st.GetGitHubAppCredentials(context.Background(), testConfig().SessionSecret)
	if err != nil {
		t.Fatalf("GetGitHubAppCredentials after second: %v", err)
	}

	if creds2.GitHubAppID != 555 || creds2.WebhookSecret != "whsec" {
		t.Fatalf("credentials overwritten: %#v", creds2)
	}
}

func TestGitHubManifestStartRejectsNonPublicAPIBaseURL(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	engine := httpapi.New(pool, testConfig()) // AppAPIBaseURL defaults to http://localhost:8080

	setupRec := postSetup(engine, map[string]string{
		"organization_name": "Acme",
		"email":             "owner@example.com",
		"display_name":      "Owner",
		"password":          "password1",
	})
	if setupRec.Code != http.StatusCreated {
		t.Fatalf("setup = %d", setupRec.Code)
	}

	var cookie *http.Cookie

	for _, c := range setupRec.Result().Cookies() {
		if c.Name == "farplane_session" {
			cookie = c
			break
		}
	}

	if cookie == nil {
		t.Fatal("missing session")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/app/manifest/start", nil)
	req.AddCookie(cookie)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("manifest start = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "public https") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestGitHubManifestStartForbiddenForMember(t *testing.T) {
	fake := &fakeGitHub{}
	engine, ownerCookie, _, pool := setupAuthedEngine(t, fake)

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(ownerCookie)

	meRec := httptest.NewRecorder()
	engine.ServeHTTP(meRec, meReq)

	var me struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}

	_ = json.Unmarshal(meRec.Body.Bytes(), &me)
	insertMemberUser(t, pool, me.Organization.ID, "member@example.com", "Member", "password1")
	memberCookie := loginCookie(t, engine, "member@example.com", "password1")

	// Engine has github forced configured — conflict for owner; use fresh engine without github.
	engine2 := httpapi.New(pool, testConfig())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/app/manifest/start", nil)
	req.AddCookie(memberCookie)

	rec := httptest.NewRecorder()
	engine2.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("member manifest start = %d, want 403 body=%s", rec.Code, rec.Body.String())
	}
}
