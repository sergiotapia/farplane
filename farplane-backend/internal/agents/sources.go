package agents

import (
	"strings"

	"github.com/farplane/farplane/farplane-backend/internal/models"
)

// Model source ids stored on lanes and used to resolve catalogs.
const (
	ModelSourceAnthropic  = "anthropic"
	ModelSourceOpenAI     = "openai"
	ModelSourceOpenRouter = "openrouter"
)

// ModelSourceOption is one choosable LLM route for an agent.
type ModelSourceOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var modelSourceLabels = map[string]string{
	ModelSourceAnthropic:  "Anthropic",
	ModelSourceOpenAI:     "OpenAI",
	ModelSourceOpenRouter: "OpenRouter",
}

// secretForModelSource maps a model source to its organization secret.
func secretForModelSource(source string) string {
	switch strings.TrimSpace(source) {
	case ModelSourceAnthropic:
		return models.SecretNameAnthropicAPIKey
	case ModelSourceOpenAI:
		return models.SecretNameOpenAIAPIKey
	case ModelSourceOpenRouter:
		return models.SecretNameOpenRouterAPIKey
	default:
		return ""
	}
}

// IsKnownModelSource reports whether source is a valid model_source value.
func IsKnownModelSource(source string) bool {
	return secretForModelSource(source) != ""
}

// ModelSourceLabel returns a UI label for a model source id.
func ModelSourceLabel(source string) string {
	if label, ok := modelSourceLabels[source]; ok {
		return label
	}
	return source
}

// supportedSourcesForAgent is the full source set an agent runtime can use.
func supportedSourcesForAgent(provider string) []string {
	switch strings.TrimSpace(provider) {
	case models.AgentProviderClaudeCode:
		return []string{ModelSourceAnthropic}
	case models.AgentProviderCodex:
		return []string{ModelSourceOpenAI}
	case models.AgentProviderOpenCode, models.AgentProviderOhMyPi:
		return []string{
			ModelSourceOpenRouter,
			ModelSourceAnthropic,
			ModelSourceOpenAI,
		}
	default:
		return nil
	}
}

// SourcesForAgent returns model sources available for an agent given set secrets.
func SourcesForAgent(provider string, setSecrets map[string]bool) []ModelSourceOption {
	supported := supportedSourcesForAgent(provider)
	out := make([]ModelSourceOption, 0, len(supported))
	for _, source := range supported {
		secret := secretForModelSource(source)
		if secret == "" || !setSecrets[secret] {
			continue
		}
		out = append(out, ModelSourceOption{
			ID:    source,
			Label: ModelSourceLabel(source),
		})
	}
	return out
}

// AgentAvailable reports whether the agent can run with any configured secret.
func AgentAvailable(provider string, setSecrets map[string]bool) bool {
	return len(SourcesForAgent(provider, setSecrets)) > 0
}

// DefaultModelSource picks a source for an agent from set secrets.
// Prefers OpenRouter for multi-provider agents (prior Farplane default), then Anthropic, then OpenAI.
func DefaultModelSource(provider string, setSecrets map[string]bool) (string, bool) {
	sources := SourcesForAgent(provider, setSecrets)
	if len(sources) == 0 {
		return "", false
	}
	return sources[0].ID, true
}

// SourceAllowedForAgent reports whether source is valid for provider when secret is set.
func SourceAllowedForAgent(provider, source string, setSecrets map[string]bool) bool {
	source = strings.TrimSpace(source)
	for _, s := range SourcesForAgent(provider, setSecrets) {
		if s.ID == source {
			return true
		}
	}
	return false
}
