// Package githubapp talks to the GitHub App API: JWT auth, installation
// tokens, repository listing, install URLs, and webhook signature checks.
package githubapp

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultAPIBaseURL  = "https://api.github.com"
	defaultWebBaseURL  = "https://github.com"
	userAgent          = "Farplane-GitHub-App"
	defaultHTTPTimeout = 30 * time.Second
)

// HTTPDoer is the subset of *http.Client used by Client (for tests).
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config configures a GitHub App API client.
type Config struct {
	AppID         int64
	Slug          string
	PrivateKeyPEM string
	WebhookSecret string
	APIBaseURL    string // optional; defaults to api.github.com
	WebBaseURL    string // optional; defaults to github.com
	HTTPClient    HTTPDoer
}

// Client calls GitHub App endpoints with an injectable transport.
type Client struct {
	appID         int64
	slug          string
	privateKey    *rsa.PrivateKey
	webhookSecret string
	apiBaseURL    string
	webBaseURL    string
	http          HTTPDoer
	now           func() time.Time
}

// New builds a Client from App credentials.
func New(cfg Config) (*Client, error) {
	if cfg.AppID <= 0 {
		return nil, errors.New("github app id is required")
	}

	if strings.TrimSpace(cfg.Slug) == "" {
		return nil, errors.New("github app slug is required")
	}

	key, err := ParsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	apiBase := strings.TrimRight(cfg.APIBaseURL, "/")
	if apiBase == "" {
		apiBase = defaultAPIBaseURL
	}

	webBase := strings.TrimRight(cfg.WebBaseURL, "/")
	if webBase == "" {
		webBase = defaultWebBaseURL
	}

	return &Client{
		appID:         cfg.AppID,
		slug:          cfg.Slug,
		privateKey:    key,
		webhookSecret: cfg.WebhookSecret,
		apiBaseURL:    apiBase,
		webBaseURL:    webBase,
		http:          httpClient,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

// InstallURL returns the GitHub URL to install the App with signed state.
func (c *Client) InstallURL(state string) string {
	u, err := url.Parse(c.webBaseURL + "/apps/" + url.PathEscape(c.slug) + "/installations/new")
	if err != nil {
		return c.webBaseURL + "/apps/" + c.slug + "/installations/new"
	}

	q := u.Query()
	if state != "" {
		q.Set("state", state)
	}

	u.RawQuery = q.Encode()

	return u.String()
}

// Installation is a GitHub App installation account summary.
type Installation struct {
	ID                  int64   `json:"id"`
	RepositorySelection string  `json:"repository_selection"`
	SuspendedAt         *string `json:"suspended_at"`
	Account             struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

// Repository is a GitHub repository visible to an installation.
type Repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
}

// GetInstallation loads installation metadata with an App JWT.
func (c *Client) GetInstallation(ctx context.Context, installationID int64) (Installation, error) {
	var out Installation
	if err := c.doAppJSON(ctx, http.MethodGet, "/app/installations/"+strconv.FormatInt(installationID, 10), nil, &out); err != nil {
		return Installation{}, err
	}

	return out, nil
}

// CreateInstallationToken mints a short-lived installation access token.
func (c *Client) CreateInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := c.doAppJSON(ctx, http.MethodPost, path, bytes.NewReader([]byte("{}")), &out); err != nil {
		return "", time.Time{}, err
	}

	return out.Token, out.ExpiresAt.UTC(), nil
}

// ListInstallationRepositories lists repos for an installation access token.
func (c *Client) ListInstallationRepositories(ctx context.Context, installationToken string) ([]Repository, error) {
	var all []Repository

	page := 1
	for {
		path := "/installation/repositories?per_page=100&page=" + strconv.Itoa(page)

		var out struct {
			Repositories []Repository `json:"repositories"`
		}
		if err := c.doTokenJSON(ctx, installationToken, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}

		all = append(all, out.Repositories...)
		if len(out.Repositories) < 100 {
			break
		}

		page++
		if page > 50 {
			return nil, errors.New("github: too many repository pages")
		}
	}

	return all, nil
}

func (c *Client) doAppJSON(ctx context.Context, method, path string, body io.Reader, dest any) error {
	jwtToken, err := c.appJWT()
	if err != nil {
		return err
	}

	return c.doJSON(ctx, "Bearer "+jwtToken, method, path, body, dest)
}

func (c *Client) doTokenJSON(ctx context.Context, token, method, path string, body io.Reader, dest any) error {
	return c.doJSON(ctx, "Bearer "+token, method, path, body, dest)
}

func (c *Client) doJSON(ctx context.Context, authorization, method, path string, body io.Reader, dest any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.apiBaseURL+path, body)
	if err != nil {
		return fmt.Errorf("github request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", authorization)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("github http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("github read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github api %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if dest == nil || len(raw) == 0 {
		return nil
	}

	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("github decode: %w", err)
	}

	return nil
}

func (c *Client) appJWT() (string, error) {
	now := c.now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
		Issuer:    strconv.FormatInt(c.appID, 10),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signed, err := token.SignedString(c.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign github app jwt: %w", err)
	}

	return signed, nil
}
