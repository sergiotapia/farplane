package agents

import "github.com/farplane/farplane/farplane-backend/internal/models"

// Static first-party model catalogs for direct Anthropic / OpenAI routes.
var (
	// Codex agent uses the Codex-only list (OpenAI-only runtime).
	codexDirectModels = []ModelOption{
		{
			ID:                     "gpt-5.1-codex",
			Label:                  "GPT-5.1 Codex",
			ReasoningEfforts:       []string{"none", "low", "medium", "high", "xhigh"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "gpt-5.1-codex-mini",
			Label:                  "GPT-5.1 Codex Mini",
			ReasoningEfforts:       []string{"none", "low", "medium", "high", "xhigh"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
	}

	// Multi-provider agents (OpenCode / oh-my-pi) may use any OpenAI-direct model.
	openaiDirectModels = []ModelOption{
		{
			ID:                     "gpt-5.1-codex",
			Label:                  "GPT-5.1 Codex",
			ReasoningEfforts:       []string{"none", "low", "medium", "high", "xhigh"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "gpt-5.1-codex-mini",
			Label:                  "GPT-5.1 Codex Mini",
			ReasoningEfforts:       []string{"none", "low", "medium", "high", "xhigh"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "gpt-5.2",
			Label:                  "GPT-5.2",
			ReasoningEfforts:       []string{"none", "low", "medium", "high", "xhigh"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "o3",
			Label:                  "o3",
			ReasoningEfforts:       []string{"low", "medium", "high"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "o4-mini",
			Label:                  "o4-mini",
			ReasoningEfforts:       []string{"low", "medium", "high"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
	}

	anthropicDirectModels = []ModelOption{
		{
			ID:                     "claude-sonnet-4-5",
			Label:                  "Claude Sonnet 4.5",
			ReasoningEfforts:       []string{"none", "low", "medium", "high"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "claude-opus-4-5",
			Label:                  "Claude Opus 4.5",
			ReasoningEfforts:       []string{"none", "low", "medium", "high"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
		{
			ID:                     "claude-haiku-4-5",
			Label:                  "Claude Haiku 4.5",
			ReasoningEfforts:       []string{"none", "low", "medium", "high"},
			DefaultReasoningEffort: stringPtr("medium"),
			SupportsReasoning:      true,
		},
	}
)

func staticModelsForSource(source string) ([]ModelOption, bool) {
	switch source {
	case ModelSourceOpenAI:
		return cloneModelOptions(openaiDirectModels), true
	case ModelSourceAnthropic:
		return cloneModelOptions(anthropicDirectModels), true
	default:
		return nil, false
	}
}

func staticModelsForAgent(provider, source string) ([]ModelOption, bool) {
	if source == ModelSourceOpenAI && provider == models.AgentProviderCodex {
		return cloneModelOptions(codexDirectModels), true
	}
	return staticModelsForSource(source)
}
