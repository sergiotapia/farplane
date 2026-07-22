package githubapp_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
)

func TestBuildManifestAndRegisterURL(t *testing.T) {
	t.Parallel()

	m := githubapp.BuildManifest("http://localhost:3000", "http://localhost:8080", "Pegas")
	if m.Name != "Farplane AI (Pegas)" {
		t.Fatalf("name = %q", m.Name)
	}
	if got := githubapp.ManifestAppName(""); got != "Farplane AI" {
		t.Fatalf("empty org name = %q", got)
	}
	if m.HookAttributes.URL != "http://localhost:8080/api/v1/github/webhook" {
		t.Fatalf("hook = %q", m.HookAttributes.URL)
	}
	if m.RedirectURL != "http://localhost:8080/api/v1/github/app/manifest/callback" {
		t.Fatalf("redirect = %q", m.RedirectURL)
	}
	if m.DefaultPermissions["contents"] != "write" {
		t.Fatalf("permissions = %#v", m.DefaultPermissions)
	}
	if len(m.DefaultEvents) != 0 {
		t.Fatalf("default_events should be empty (installation events are automatic), got %#v", m.DefaultEvents)
	}
	if githubapp.ManifestRegisterURL("") != "https://github.com/settings/apps/new" {
		t.Fatalf("personal register url unexpected")
	}
	if got := githubapp.ManifestRegisterURL("acme-co"); got != "https://github.com/organizations/acme-co/settings/apps/new" {
		t.Fatalf("org register url = %q", got)
	}
}

func TestConvertManifest(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || !strings.Contains(req.URL.Path, "/app-manifests/abc/conversions") {
			t.Fatalf("unexpected %s %s", req.Method, req.URL.String())
		}
		body := `{"id":42,"slug":"farplane","name":"Farplane","client_id":"cid","client_secret":"csec","webhook_secret":"whsec","pem":"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"}`
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	app, err := githubapp.ConvertManifest(context.Background(), transport, "https://api.github.test", "abc")
	if err != nil {
		t.Fatalf("ConvertManifest: %v", err)
	}
	if app.ID != 42 || app.Slug != "farplane" || app.WebhookSecret != "whsec" || app.PEM == "" {
		t.Fatalf("app = %+v", app)
	}
}

func TestConvertManifestRequiresWebhookSecret(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"id":42,"slug":"farplane","name":"Farplane","client_id":"cid","client_secret":"csec","webhook_secret":"","pem":"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----"}`
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	_, err := githubapp.ConvertManifest(context.Background(), transport, "https://api.github.test", "abc")
	if err == nil {
		t.Fatal("expected error for empty webhook_secret")
	}
}

func TestManifestAppNameTruncatesRunes(t *testing.T) {
	t.Parallel()

	// Multi-byte runes must not be sliced mid-character (GitHub limit is 34 chars).
	name := githubapp.ManifestAppName("組織組織組織組織組織組織組織組織")
	if got := len([]rune(name)); got > 34 {
		t.Fatalf("rune len = %d, want <= 34 (%q)", got, name)
	}
	if !strings.HasPrefix(name, "Farplane AI (") {
		t.Fatalf("name = %q", name)
	}
}
