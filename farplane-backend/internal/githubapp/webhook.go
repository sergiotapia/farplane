package githubapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// VerifyWebhookSignature checks X-Hub-Signature-256 (sha256=<hex>).
func (c *Client) VerifyWebhookSignature(body []byte, signatureHeader string) bool {
	if c.webhookSecret == "" || signatureHeader == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	_, _ = mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// WebhookInstallationPayload is the subset of installation webhooks we handle.
type WebhookInstallationPayload struct {
	Action       string `json:"action"`
	Installation struct {
		ID                  int64   `json:"id"`
		RepositorySelection string  `json:"repository_selection"`
		SuspendedAt         *string `json:"suspended_at"`
		Account             struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
	} `json:"installation"`
	RepositoriesAdded []struct {
		ID            int64  `json:"id"`
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
		Private       bool   `json:"private"`
		HTMLURL       string `json:"html_url"`
	} `json:"repositories_added"`
	RepositoriesRemoved []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repositories_removed"`
}

// ParseWebhookInstallation decodes an installation or installation_repositories body.
func ParseWebhookInstallation(body []byte) (WebhookInstallationPayload, error) {
	var payload WebhookInstallationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookInstallationPayload{}, fmt.Errorf("decode webhook: %w", err)
	}
	return payload, nil
}
