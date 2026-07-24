package githubapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Manifest is the JSON body posted to GitHub's "new App" form.
type Manifest struct {
	Name               string            `json:"name"`
	URL                string            `json:"url"`
	HookAttributes     ManifestHook      `json:"hook_attributes"`
	RedirectURL        string            `json:"redirect_url"`
	CallbackURLs       []string          `json:"callback_urls"`
	SetupURL           string            `json:"setup_url"`
	Description        string            `json:"description,omitempty"`
	Public             bool              `json:"public"`
	DefaultPermissions map[string]string `json:"default_permissions"`
	// DefaultEvents lists optional webhook subscriptions. Do not include
	// installation / installation_repositories — GitHub rejects those in
	// manifests and delivers them to Apps automatically.
	DefaultEvents []string `json:"default_events,omitempty"`
	SetupOnUpdate bool     `json:"setup_on_update"`
}

// ManifestHook configures the App webhook.
type ManifestHook struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// ManifestApp is the credential payload returned by the conversions endpoint.
type ManifestApp struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	WebhookSecret string `json:"webhook_secret"`
	PEM           string `json:"pem"`
}

// BuildManifest returns the Farplane GitHub App manifest for this install's URLs.
// farplaneOrganizationName is used to make the App name unique: GitHub App names
// are global (or reserved), so each self-hosted install needs its own name.
// The operator can still edit the name on GitHub's create form.
func BuildManifest(appBaseURL, apiBaseURL, farplaneOrganizationName string) Manifest {
	appBaseURL = strings.TrimRight(appBaseURL, "/")
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")

	return Manifest{
		Name: ManifestAppName(farplaneOrganizationName),
		URL:  appBaseURL,
		HookAttributes: ManifestHook{
			URL:    apiBaseURL + "/api/v1/github/webhook",
			Active: true,
		},
		RedirectURL:  apiBaseURL + "/api/v1/github/app/manifest/callback",
		CallbackURLs: []string{apiBaseURL + "/api/v1/github/callback"},
		SetupURL:     apiBaseURL + "/api/v1/github/callback",
		Description:  "Farplane connects this install to GitHub repositories for Projects and Lanes.",
		Public:       true,
		DefaultPermissions: map[string]string{
			"contents":      "write",
			"pull_requests": "write",
			"metadata":      "read",
		},
		SetupOnUpdate: true,
	}
}

// ManifestAppName builds a unique-ish GitHub App display name for this install.
// GitHub App names must be unique across GitHub (plain "Farplane" is reserved).
func ManifestAppName(farplaneOrganizationName string) string {
	org := strings.TrimSpace(farplaneOrganizationName)

	name := "Farplane AI"
	if org != "" {
		name = "Farplane AI (" + org + ")"
	}
	// GitHub App names are limited to 34 characters (count runes, not bytes).
	const maxLen = 34

	runes := []rune(name)
	if len(runes) > maxLen {
		name = strings.TrimSpace(string(runes[:maxLen]))
	}

	return name
}

// ManifestRegisterURL is where the browser POSTs the manifest form.
// githubOrganizationLogin empty → personal account; otherwise that org.
func ManifestRegisterURL(githubOrganizationLogin string) string {
	org := strings.TrimSpace(githubOrganizationLogin)
	if org == "" {
		return defaultWebBaseURL + "/settings/apps/new"
	}

	return defaultWebBaseURL + "/organizations/" + url.PathEscape(org) + "/settings/apps/new"
}

// ConvertManifest exchanges a temporary manifest code for App credentials.
func ConvertManifest(ctx context.Context, httpClient HTTPDoer, apiBaseURL, code string) (ManifestApp, error) { //nolint:gocyclo // multi-branch orchestration; keep under threshold when rewriting
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	apiBaseURL = strings.TrimRight(apiBaseURL, "/")
	if apiBaseURL == "" {
		apiBaseURL = defaultAPIBaseURL
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return ManifestApp{}, errors.New("manifest code is empty")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		apiBaseURL+"/app-manifests/"+url.PathEscape(code)+"/conversions",
		nil,
	)
	if err != nil {
		return ManifestApp{}, fmt.Errorf("manifest convert request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return ManifestApp{}, fmt.Errorf("manifest convert http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ManifestApp{}, fmt.Errorf("manifest convert read: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ManifestApp{}, fmt.Errorf("manifest convert: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out ManifestApp
	if err := json.Unmarshal(raw, &out); err != nil {
		return ManifestApp{}, fmt.Errorf("manifest convert decode: %w", err)
	}

	if out.ID <= 0 || out.PEM == "" || out.Slug == "" || strings.TrimSpace(out.WebhookSecret) == "" {
		return ManifestApp{}, errors.New("manifest convert: incomplete app credentials")
	}

	return out, nil
}
