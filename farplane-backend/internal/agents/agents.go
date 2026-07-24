// Package agents defines the Lane agent catalog and secret gating rules.
package agents

import "github.com/farplane/farplane/farplane-backend/internal/models"

// Agent describes one choosable Lane agent provider.
type Agent struct {
	Provider       string              `json:"provider"`
	Label          string              `json:"label"`
	RequiredSecret string              `json:"required_secret"`
	Available      bool                `json:"available"`
	ModelSources   []ModelSourceOption `json:"model_sources"`
}

// Catalog is the fixed v1 agent list.
var Catalog = []Agent{
	{
		Provider:       models.AgentProviderClaudeCode,
		Label:          "Claude Code",
		RequiredSecret: models.SecretNameAnthropicAPIKey,
	},
	{
		Provider:       models.AgentProviderCodex,
		Label:          "Codex",
		RequiredSecret: models.SecretNameOpenAIAPIKey,
	},
	{
		Provider:       models.AgentProviderOpenCode,
		Label:          "OpenCode",
		RequiredSecret: models.SecretNameOpenRouterAPIKey,
	},
	{
		Provider:       models.AgentProviderOhMyPi,
		Label:          "oh-my-pi (omp)",
		RequiredSecret: models.SecretNameOpenRouterAPIKey,
	},
}

// WellKnownSecretNames are the Settings → Secrets fields for v1.
var WellKnownSecretNames = []string{
	models.SecretNameAnthropicAPIKey,
	models.SecretNameOpenAIAPIKey,
	models.SecretNameOpenRouterAPIKey,
}

// SecretLabel returns a UI label for a well-known secret name.
func SecretLabel(name string) string {
	switch name {
	case models.SecretNameAnthropicAPIKey:
		return "Claude API key"
	case models.SecretNameOpenAIAPIKey:
		return "OpenAI API key"
	case models.SecretNameOpenRouterAPIKey:
		return "OpenRouter API key"
	default:
		return name
	}
}

// Availability marks each catalog entry available when any required model
// source secret is set, and attaches the available model_sources list.
func Availability(setSecrets map[string]bool) []Agent {
	out := make([]Agent, 0, len(Catalog))
	for _, a := range Catalog {
		sources := SourcesForAgent(a.Provider, setSecrets)
		a.ModelSources = sources
		a.Available = len(sources) > 0
		out = append(out, a)
	}

	return out
}

// RequiredSecretFor returns a primary secret name for a provider (display / legacy).
// Multi-provider agents still list OpenRouter as primary; use AgentAvailable for gating.
func RequiredSecretFor(provider string) string {
	for _, a := range Catalog {
		if a.Provider == provider {
			return a.RequiredSecret
		}
	}

	return ""
}

// IsKnownProvider reports whether provider is in the catalog.
func IsKnownProvider(provider string) bool {
	return RequiredSecretFor(provider) != ""
}
