package features_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/farplane/farplane/farplane-backend/internal/config"
	"github.com/farplane/farplane/farplane-backend/internal/db"
	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
	"github.com/farplane/farplane/farplane-backend/internal/httpapi"
)

type apiWorld struct {
	t              *testing.T
	pool           *pgxpool.Pool
	engine         http.Handler
	cfg            config.Config
	sessionCookie  *http.Cookie
	lastRec        *httptest.ResponseRecorder
	manifestState  string
	manifestActive bool
}

func testDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}

	return "postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable"
}

func defaultConfig() config.Config {
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

func openMigratedTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := testDatabaseURL()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}

	t.Cleanup(func() { pool.Close() })

	if err := db.MigrateReset(url); err != nil {
		t.Fatalf("MigrateReset: %v", err)
	}

	if err := db.MigrateUp(url); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}

	return pool
}

func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	der := x509.MarshalPKCS1PrivateKey(key)

	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

func (w *apiWorld) resetCleanAPI() error {
	w.pool = openMigratedTestDB(w.t)
	w.cfg = defaultConfig()
	w.engine = httpapi.New(w.pool, w.cfg)
	w.sessionCookie = nil
	w.lastRec = nil
	w.manifestState = ""
	w.manifestActive = false

	return nil
}

func (w *apiWorld) resetCleanAPIPublicBase() error {
	if err := w.resetCleanAPI(); err != nil {
		return err
	}

	w.cfg.AppAPIBaseURL = "https://farplane-test.example.com"
	w.engine = httpapi.New(w.pool, w.cfg)

	return nil
}

func (w *apiWorld) serve(req *http.Request) {
	if w.sessionCookie != nil {
		req.AddCookie(w.sessionCookie)
	}

	rec := httptest.NewRecorder()
	w.engine.ServeHTTP(rec, req)

	w.lastRec = rec
	for _, c := range rec.Result().Cookies() {
		if c.Name == "farplane_session" {
			if c.MaxAge < 0 || c.Value == "" {
				w.sessionCookie = nil
			} else {
				w.sessionCookie = c
			}
		}
	}
}

func (w *apiWorld) setupOrganization(org, email, password string) error {
	body, _ := json.Marshal(map[string]string{
		"organization_name": org,
		"email":             email,
		"display_name":      "Owner",
		"password":          password,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/setup", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w.serve(req)

	return nil
}

func (w *apiWorld) givenOrganization(org, email, password string) error {
	if err := w.setupOrganization(org, email, password); err != nil {
		return err
	}

	if w.lastRec.Code != http.StatusCreated {
		return fmt.Errorf("setup status = %d body=%s", w.lastRec.Code, w.lastRec.Body.String())
	}

	return nil
}

func (w *apiWorld) logIn(email, password string) error {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w.serve(req)

	return nil
}

func (w *apiWorld) logOut() error {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w.serve(req)

	return nil
}

func (w *apiWorld) getPath(path string) error {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w.serve(req)

	return nil
}

func (w *apiWorld) responseStatusIs(code int) error {
	if w.lastRec == nil {
		return errors.New("no response recorded")
	}

	if w.lastRec.Code != code {
		return fmt.Errorf("status = %d, want %d; body=%s", w.lastRec.Code, code, w.lastRec.Body.String())
	}

	return nil
}

func (w *apiWorld) haveSessionCookie() error {
	if w.sessionCookie == nil || w.sessionCookie.Value == "" {
		return errors.New("expected farplane_session cookie")
	}

	return nil
}

func jsonField(raw []byte, path string) (any, error) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}

	cur := root
	for part := range strings.SplitSeq(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q not an object at %q", path, part)
		}

		cur, ok = obj[part]
		if !ok {
			return nil, fmt.Errorf("missing field %q in %q", part, path)
		}
	}

	return cur, nil
}

func (w *apiWorld) jsonFieldIs(path, want string) error {
	got, err := jsonField(w.lastRec.Body.Bytes(), path)
	if err != nil {
		return err
	}

	switch v := got.(type) {
	case string:
		if v != want {
			return fmt.Errorf("%s = %q, want %q", path, v, want)
		}
	case bool:
		wantBool := want == "true"
		if want != "true" && want != "false" {
			return fmt.Errorf("want %q is not a bool literal", want)
		}

		if v != wantBool {
			return fmt.Errorf("%s = %v, want %v", path, v, wantBool)
		}
	default:
		if fmt.Sprint(v) != want {
			return fmt.Errorf("%s = %#v, want %q", path, v, want)
		}
	}

	return nil
}

func (w *apiWorld) membersInclude(email string) error {
	var body struct {
		Members []struct {
			Email string `json:"email"`
		} `json:"members"`
	}
	if err := json.Unmarshal(w.lastRec.Body.Bytes(), &body); err != nil {
		return fmt.Errorf("decode members: %w; body=%s", err, w.lastRec.Body.String())
	}

	for _, m := range body.Members {
		if m.Email == email {
			return nil
		}
	}

	return fmt.Errorf("members %#v missing %s", body.Members, email)
}

func (w *apiWorld) enableFakeManifest() error {
	pemKey := testRSAPrivateKeyPEM(w.t)
	w.engine = httpapi.New(
		w.pool,
		w.cfg,
		httpapi.WithManifestConvert(func(ctx context.Context, code string) (githubapp.ManifestApp, error) {
			_ = ctx

			if code != "manifest-code" {
				return githubapp.ManifestApp{}, fmt.Errorf("unexpected code %q", code)
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
	w.manifestActive = true

	return nil
}

func (w *apiWorld) startManifest() error {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/app/manifest/start", nil)
	w.serve(req)

	if w.lastRec.Code == http.StatusOK {
		var body struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(w.lastRec.Body.Bytes(), &body); err != nil {
			return err
		}

		w.manifestState = body.State
	}

	return nil
}

func (w *apiWorld) completeManifest(code string) error {
	if !w.manifestActive {
		return errors.New("fake manifest converter not enabled")
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/github/app/manifest/callback?code="+code+"&state="+w.manifestState,
		nil,
	)
	// Callback is unauthenticated in the product flow.
	cookie := w.sessionCookie
	w.sessionCookie = nil
	w.serve(req)
	w.sessionCookie = cookie

	return nil
}

func (w *apiWorld) redirectContains(fragment string) error {
	loc := w.lastRec.Header().Get("Location")
	if !strings.Contains(loc, fragment) {
		return fmt.Errorf("Location = %q, want substring %q", loc, fragment)
	}

	return nil
}

func initializeScenario(t *testing.T) func(ctx *godog.ScenarioContext) {
	return func(ctx *godog.ScenarioContext) {
		var w *apiWorld

		ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
			_ = sc
			w = &apiWorld{t: t}

			return ctx, nil
		})

		// Closures must read w at call time; method values capture the nil receiver.
		ctx.Step(`^a clean Farplane API$`, func() error { return w.resetCleanAPI() })
		ctx.Step(`^a clean Farplane API with a public API base URL$`, func() error {
			return w.resetCleanAPIPublicBase()
		})
		ctx.Step(`^I set up an organization "([^"]*)" as "([^"]*)" with password "([^"]*)"$`,
			func(org, email, password string) error { return w.setupOrganization(org, email, password) })
		ctx.Step(`^an organization "([^"]*)" owned by "([^"]*)" with password "([^"]*)"$`,
			func(org, email, password string) error { return w.givenOrganization(org, email, password) })
		ctx.Step(`^I log in as "([^"]*)" with password "([^"]*)"$`,
			func(email, password string) error { return w.logIn(email, password) })
		ctx.Step(`^I log out$`, func() error { return w.logOut() })
		ctx.Step(`^I get "([^"]*)"$`, func(path string) error { return w.getPath(path) })
		ctx.Step(`^the response status is (\d+)$`, func(code int) error { return w.responseStatusIs(code) })
		ctx.Step(`^I have a session cookie$`, func() error { return w.haveSessionCookie() })
		ctx.Step(`^the JSON field "([^"]*)" is "([^"]*)"$`,
			func(path, want string) error { return w.jsonFieldIs(path, want) })
		ctx.Step(`^the JSON field "([^"]*)" is (true|false)$`,
			func(path, want string) error { return w.jsonFieldIs(path, want) })
		ctx.Step(`^the organization members include "([^"]*)"$`,
			func(email string) error { return w.membersInclude(email) })
		ctx.Step(`^a fake GitHub App manifest converter$`, func() error { return w.enableFakeManifest() })
		ctx.Step(`^I start the GitHub App manifest flow$`, func() error { return w.startManifest() })
		ctx.Step(`^I complete the GitHub App manifest callback with code "([^"]*)"$`,
			func(code string) error { return w.completeManifest(code) })
		ctx.Step(`^the redirect location contains "([^"]*)"$`,
			func(fragment string) error { return w.redirectContains(fragment) })
	}
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "farplane-backend",
		ScenarioInitializer: initializeScenario(t),
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"."},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("godog suite failed")
	}
}
