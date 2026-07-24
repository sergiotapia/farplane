package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/db"
	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
)

func testConfig() config.Config {
	return config.Config{
		AppBaseURL:           "http://localhost:3000",
		AppAPIBaseURL:        "http://localhost:8080",
		GoogleRedirectURL:    "http://localhost:8080/api/v1/auth/google/callback",
		GitHubAppCallbackURL: "http://localhost:8080/api/v1/github/callback",
		SessionSecret:        "test-session-secret",
		SessionCookieSecure:  false,
		SessionTTL:           24 * time.Hour,
	}
}

func TestHealthLiveness(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	httpapi.New(nil, testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("status = %q, want %q", body["status"], "ok")
	}
}

func TestReadyWithoutPool(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	httpapi.New(nil, testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["status"] != "unavailable" {
		t.Fatalf("status = %q, want %q", body["status"], "unavailable")
	}

	if body["database"] != "missing" {
		t.Fatalf("database = %q, want %q", body["database"], "missing")
	}
}

func TestReadyWithPool(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}
	defer pool.Close()

	if err := db.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	httpapi.New(pool, testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("status = %q, want %q", body["status"], "ok")
	}

	if body["database"] != "up" {
		t.Fatalf("database = %q, want %q", body["database"], "up")
	}
}

func TestHello(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hello", nil)
	rec := httptest.NewRecorder()
	httpapi.New(nil, testConfig()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body["message"] != "farplane" {
		t.Fatalf("message = %q, want %q", body["message"], "farplane")
	}
}

func TestCORSAllowsSPAOrigin(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{"http://localhost:3000", "http://127.0.0.1:3000"} {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/hello", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")

		rec := httptest.NewRecorder()
		httpapi.New(nil, testConfig()).ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, origin)
		}
	}
}
