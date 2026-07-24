package githubapp_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/farplane/farplane/farplane-backend/internal/githubapp"
)

func signWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)

	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	der := x509.MarshalPKCS1PrivateKey(key)

	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestInstallURLAndWebhookSignature(t *testing.T) {
	t.Parallel()

	pemKey := testPrivateKeyPEM(t)

	client, err := githubapp.New(githubapp.Config{
		AppID:         12345,
		Slug:          "farplane",
		PrivateKeyPEM: pemKey,
		WebhookSecret: "hook-secret",
		WebBaseURL:    "https://github.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	url := client.InstallURL("abc.state")
	if !strings.Contains(url, "/apps/farplane/installations/new") {
		t.Fatalf("InstallURL = %q", url)
	}

	if !strings.Contains(url, "state=abc.state") {
		t.Fatalf("InstallURL missing state: %q", url)
	}

	body := []byte(`{"action":"created"}`)

	macOK := signWebhook("hook-secret", body)
	if !client.VerifyWebhookSignature(body, macOK) {
		t.Fatal("expected valid signature")
	}

	if client.VerifyWebhookSignature(body, "sha256=deadbeef") {
		t.Fatal("expected invalid signature")
	}
}

func TestCreateInstallationTokenAndListRepos(t *testing.T) {
	t.Parallel()

	pemKey := testPrivateKeyPEM(t)

	var sawPaths []string

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sawPaths = append(sawPaths, req.Method+" "+req.URL.Path)

		auth := req.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Fatalf("missing bearer auth on %s", req.URL.Path)
		}

		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/access_tokens"):
			body := `{"token":"ghs_test","expires_at":"2030-01-02T03:04:05Z"}`

			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		case req.Method == http.MethodGet && req.URL.Path == "/installation/repositories":
			body := `{"repositories":[{"id":99,"full_name":"acme/web","default_branch":"main","private":true,"html_url":"https://github.com/acme/web"}]}`

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		default:
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})

	client, err := githubapp.New(githubapp.Config{
		AppID:         99,
		Slug:          "farplane",
		PrivateKeyPEM: pemKey,
		WebhookSecret: "secret",
		APIBaseURL:    "https://api.github.test",
		HTTPClient:    transport,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()

	token, expires, err := client.CreateInstallationToken(ctx, 777)
	if err != nil {
		t.Fatalf("CreateInstallationToken: %v", err)
	}

	if token != "ghs_test" {
		t.Fatalf("token = %q", token)
	}

	if !expires.Equal(time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("expires = %v", expires)
	}

	repos, err := client.ListInstallationRepositories(ctx, token)
	if err != nil {
		t.Fatalf("ListInstallationRepositories: %v", err)
	}

	if len(repos) != 1 || repos[0].FullName != "acme/web" || repos[0].ID != 99 {
		t.Fatalf("repos = %+v", repos)
	}

	if len(sawPaths) != 2 {
		t.Fatalf("paths = %v", sawPaths)
	}
}
