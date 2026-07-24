package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/db"
	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
)

func testDatabaseURL() string {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		return "postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable"
	}

	return url
}

func openMigratedTestDB(t *testing.T, reset bool) *pgxpool.Pool {
	t.Helper()

	url := testDatabaseURL()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}

	t.Cleanup(func() { pool.Close() })

	if reset {
		if err := db.MigrateReset(url); err != nil {
			t.Fatalf("MigrateReset: %v", err)
		}
	}

	if err := db.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	return pool
}

func postSetup(engine http.Handler, body map[string]string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	return rec
}

func TestSetupStatusWhenEmptyAndGoogleStartWithoutCreds(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	engine := httpapi.New(pool, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", rec.Code, rec.Body.String())
	}

	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if status["needs_setup"] != true {
		t.Fatalf("needs_setup = %#v, want true", status["needs_setup"])
	}

	if status["google_oauth_configured"] != false {
		t.Fatalf("google_oauth_configured = %#v, want false", status["google_oauth_configured"])
	}

	if status["setup_token_required"] != false {
		t.Fatalf("setup_token_required = %#v, want false", status["setup_token_required"])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/start?intent=login", nil)
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("google start status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSetupStatusReportsGoogleOAuthConfiguredFromConfig(t *testing.T) {
	pool := openMigratedTestDB(t, true)

	cfg := testConfig()
	cfg.GoogleClientID = "test-client-id"
	cfg.GoogleClientSecret = "test-client-secret"
	engine := httpapi.New(pool, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", rec.Code, rec.Body.String())
	}

	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if status["needs_setup"] != true {
		t.Fatalf("needs_setup = %#v, want true", status["needs_setup"])
	}

	if status["google_oauth_configured"] != true {
		t.Fatalf("google_oauth_configured = %#v, want true", status["google_oauth_configured"])
	}
}

func TestPasswordSetupCreatesOwnerMembershipAndSecondFails(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	engine := httpapi.New(pool, testConfig())

	body := map[string]string{
		"organization_name": "Acme Co",
		"email":             "owner@example.com",
		"display_name":      "Owner",
		"password":          "password1",
	}

	rec := postSetup(engine, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d body=%s", rec.Code, rec.Body.String())
	}

	var setupResp struct {
		User struct {
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		} `json:"user"`
		Organization struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &setupResp); err != nil {
		t.Fatalf("decode setup: %v", err)
	}

	if setupResp.User.Email != "owner@example.com" {
		t.Fatalf("email = %q, want owner@example.com", setupResp.User.Email)
	}

	if setupResp.Organization.Name != "Acme Co" {
		t.Fatalf("organization name = %q, want Acme Co", setupResp.Organization.Name)
	}

	if setupResp.Organization.Role != "owner" {
		t.Fatalf("role = %q, want owner", setupResp.Organization.Role)
	}

	cookie := rec.Result().Cookies()

	var sessionCookie *http.Cookie

	for _, c := range cookie {
		if c.Name == "farplane_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected farplane_session cookie")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)

	meRec := httptest.NewRecorder()
	engine.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", meRec.Code, meRec.Body.String())
	}

	var meResp struct {
		Organization struct {
			Role string `json:"role"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("decode me: %v", err)
	}

	if meResp.Organization.Role != "owner" {
		t.Fatalf("me role = %q, want owner", meResp.Organization.Role)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	statusRec := httptest.NewRecorder()
	engine.ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status after setup = %d body=%s", statusRec.Code, statusRec.Body.String())
	}

	var status map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}

	if status["needs_setup"] != false {
		t.Fatalf("needs_setup after setup = %#v, want false", status["needs_setup"])
	}

	rec2 := postSetup(engine, body)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d, want 409; body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestConcurrentPasswordSetupOnlyOneSucceeds(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	engine := httpapi.New(pool, testConfig())

	const workers = 8

	results := make([]int, workers)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func() {
			defer wg.Done()

			body := map[string]string{
				"organization_name": "Race Org",
				"email":             "race-owner-" + strconv.Itoa(i) + "@example.com",
				"display_name":      "Racer",
				"password":          "password1",
			}
			results[i] = postSetup(engine, body).Code
		}()
	}

	wg.Wait()

	created := 0
	conflict := 0

	for _, code := range results {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected setup status %d among %#v", code, results)
		}
	}

	if created != 1 {
		t.Fatalf("created count = %d, want 1; codes=%#v", created, results)
	}

	if conflict != workers-1 {
		t.Fatalf("conflict count = %d, want %d; codes=%#v", conflict, workers-1, results)
	}

	// After the race winner, status must show setup complete; google flag still follows config.
	cfg := testConfig()
	cfg.GoogleClientID = "id"
	cfg.GoogleClientSecret = "secret"
	statusEngine := httpapi.New(pool, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	statusEngine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", rec.Code, rec.Body.String())
	}

	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if status["needs_setup"] != false {
		t.Fatalf("needs_setup = %#v, want false", status["needs_setup"])
	}

	if status["google_oauth_configured"] != true {
		t.Fatalf("google_oauth_configured = %#v, want true", status["google_oauth_configured"])
	}
}
