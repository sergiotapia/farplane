package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
)

func TestLoginLogoutRoundTrip(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	engine := httpapi.New(pool, testConfig())

	setupRec := postSetup(engine, map[string]string{
		"organization_name": "Acme Co",
		"email":             "owner@example.com",
		"display_name":      "Owner",
		"password":          "password1",
	})
	if setupRec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d body=%s", setupRec.Code, setupRec.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)

	for _, c := range setupRec.Result().Cookies() {
		if c.Name == "farplane_session" {
			logoutReq.AddCookie(c)
		}
	}

	logoutRec := httptest.NewRecorder()
	engine.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", logoutRec.Code, logoutRec.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)

	for _, c := range setupRec.Result().Cookies() {
		if c.Name == "farplane_session" {
			meReq.AddCookie(c)
		}
	}

	meRec := httptest.NewRecorder()
	engine.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401", meRec.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{
		"email":    "owner@example.com",
		"password": "password1",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")

	loginRec := httptest.NewRecorder()
	engine.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginRec.Code, loginRec.Body.String())
	}

	var loginCookie *http.Cookie

	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "farplane_session" {
			loginCookie = c
		}
	}

	if loginCookie == nil || loginCookie.Value == "" {
		t.Fatal("expected farplane_session cookie after login")
	}

	meReq = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(loginCookie)

	meRec = httptest.NewRecorder()
	engine.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("me after login status = %d body=%s", meRec.Code, meRec.Body.String())
	}

	badBody, _ := json.Marshal(map[string]string{
		"email":    "owner@example.com",
		"password": "wrong-password",
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(badBody))
	badReq.Header.Set("Content-Type", "application/json")

	badRec := httptest.NewRecorder()
	engine.ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want 401", badRec.Code)
	}
}

func TestGoogleLoginRedirectsToSetupWhenNeeded(t *testing.T) {
	pool := openMigratedTestDB(t, true)
	cfg := testConfig()
	cfg.GoogleClientID = "test-client-id"
	cfg.GoogleClientSecret = "test-client-secret"
	engine := httpapi.New(pool, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/start?intent=login", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", rec.Code, rec.Body.String())
	}

	if loc := rec.Header().Get("Location"); loc != cfg.AppBaseURL+"/setup" {
		t.Fatalf("Location = %q, want setup", loc)
	}
}
